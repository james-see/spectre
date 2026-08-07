package deco

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var sysauthRe = regexp.MustCompile(`(?i)sysauth=([a-f0-9]+)`)

// Client talks to a TP-Link Deco master local admin API.
type Client struct {
	host     string
	username string
	password string
	http     *http.Client

	mu sync.RWMutex // guards session + crypto fields; posts hold RLock so they can run in parallel

	aesKey   string
	aesKeyB  []byte
	aesIV    string
	aesIVB   []byte
	passRSAN string
	passRSAE string
	signRSAN string
	signRSAE string
	seq      *int
	stok     string
	sysauth  string
}

func NewClient(host, username, password string) *Client {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host != "" && !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		host:     host,
		username: username,
		password: password,
		http: &http.Client{
			Timeout: 15 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // LAN Deco often self-signed
			},
			// Keep cookies across scheme-change redirects (some firmwares 307 http→https).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (c *Client) clearAuth() {
	c.seq = nil
	c.stok = ""
	c.sysauth = ""
}

func (c *Client) captureSysauth(resp *http.Response) {
	for _, sc := range resp.Header.Values("Set-Cookie") {
		if m := sysauthRe.FindStringSubmatch(sc); len(m) == 2 {
			c.sysauth = m[1]
			return
		}
	}
	// Fallback: jar cookies for this host.
	if c.http.Jar != nil && resp.Request != nil && resp.Request.URL != nil {
		for _, ck := range c.http.Jar.Cookies(resp.Request.URL) {
			if strings.EqualFold(ck.Name, "sysauth") && ck.Value != "" {
				c.sysauth = ck.Value
				return
			}
		}
	}
}

// SessionInfo is for diagnostics (no secrets).
func (c *Client) SessionInfo() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return fmt.Sprintf("stok=%v sysauth_len=%d seq=%v", c.stok != "", len(c.sysauth), c.seq != nil)
}

func (c *Client) ensureAES() error {
	if c.aesKey != "" {
		return nil
	}
	key, keyB, err := randomAESDigitKey()
	if err != nil {
		return err
	}
	iv, ivB, err := randomAESDigitKey()
	if err != nil {
		return err
	}
	c.aesKey, c.aesKeyB = key, keyB
	c.aesIV, c.aesIVB = iv, ivB
	return nil
}

func (c *Client) post(path string, form url.Values, body any, signed bool) (map[string]any, error) {
	// Snapshot session under RLock so parallel ListClients can overlap on the wire.
	c.mu.RLock()
	host := c.host
	sysauth := c.sysauth
	var payload []byte
	var err error
	if signed {
		payload, err = c.encodePayload(body)
	} else {
		payload, err = json.Marshal(body)
	}
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	u := host + path
	if len(form) > 0 {
		u += "?" + form.Encode()
	}
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	// HA / Deco web UI use application/json even when body is sign=&data=.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", host+"/webpages/login.html")
	if sysauth != "" {
		// Explicit cookie — some firmwares ignore jar path scoping for ;stok= URLs.
		req.Header.Set("Cookie", "sysauth="+sysauth)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.mu.Lock()
		c.clearAuth()
		c.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()

	c.mu.Lock()
	// Follow permanent switch to https if redirected from http.
	if resp.Request != nil && resp.Request.URL != nil &&
		strings.HasPrefix(c.host, "http://") &&
		resp.Request.URL.Scheme == "https" {
		c.host = "https://" + strings.TrimPrefix(c.host, "http://")
	}
	c.captureSysauth(resp)
	if resp.StatusCode == 401 {
		c.clearAuth()
	}
	c.mu.Unlock()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		hint := ""
		if resp.StatusCode == 403 {
			hint = " (Deco often returns 403 when another admin/manager session is active — log out of the Deco app/web UI, or use a dedicated manager account)"
		}
		return nil, fmt.Errorf("http %d%s: %s", resp.StatusCode, hint, truncate(strings.TrimSpace(string(raw)), 120))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("json: %w (body=%q)", err, truncate(string(raw), 200))
	}
	if ec, ok := out["error_code"]; ok {
		switch v := ec.(type) {
		case float64:
			if v != 0 {
				return nil, fmt.Errorf("error_code=%v", v)
			}
		case string:
			if v != "" && v != "0" {
				return nil, fmt.Errorf("error_code=%s", v)
			}
		}
	}
	return out, nil
}

func (c *Client) fetchKeys() error {
	out, err := c.post("/cgi-bin/luci/;stok=/login", url.Values{"form": {"keys"}},
		map[string]any{"operation": "read"}, false)
	if err != nil {
		return fmt.Errorf("fetch keys: %w", err)
	}
	result, _ := out["result"].(map[string]any)
	pw, _ := result["password"].([]any)
	if len(pw) < 2 {
		return fmt.Errorf("fetch keys: missing password key")
	}
	c.mu.Lock()
	c.passRSAN = fmt.Sprint(pw[0])
	c.passRSAE = fmt.Sprint(pw[1])
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchAuth() error {
	out, err := c.post("/cgi-bin/luci/;stok=/login", url.Values{"form": {"auth"}},
		map[string]any{"operation": "read"}, false)
	if err != nil {
		return fmt.Errorf("fetch auth: %w", err)
	}
	result, _ := out["result"].(map[string]any)
	key, _ := result["key"].([]any)
	if len(key) < 2 {
		return fmt.Errorf("fetch auth: missing sign key")
	}
	seqF, ok := result["seq"].(float64)
	if !ok {
		return fmt.Errorf("fetch auth: missing seq")
	}
	seq := int(seqF)
	c.mu.Lock()
	c.signRSAN = fmt.Sprint(key[0])
	c.signRSAE = fmt.Sprint(key[1])
	c.seq = &seq
	c.mu.Unlock()
	return nil
}

func (c *Client) Login() error {
	c.mu.Lock()
	err := c.ensureAES()
	needKeys := c.passRSAN == ""
	needAuth := c.seq == nil
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if needKeys {
		if err := c.fetchKeys(); err != nil {
			return err
		}
	}
	if needAuth {
		if err := c.fetchAuth(); err != nil {
			return err
		}
	}

	c.mu.RLock()
	passN, passE := c.passRSAN, c.passRSAE
	password := c.password
	c.mu.RUnlock()
	pwEnc, err := rsaEncryptTPLink(passN, passE, []byte(password))
	if err != nil {
		return err
	}
	payload := map[string]any{
		"params":    map[string]any{"password": pwEnc},
		"operation": "login",
	}
	out, err := c.post("/cgi-bin/luci/;stok=/login", url.Values{"form": {"login"}}, payload, true)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	dataStr, _ := out["data"].(string)
	data, err := c.decryptData(dataStr)
	if err != nil {
		return fmt.Errorf("login decrypt: %w", err)
	}
	ec := asInt(data["error_code"])
	if ec != 0 {
		c.mu.Lock()
		c.clearAuth()
		c.mu.Unlock()
		return fmt.Errorf("login error_code=%d result=%v", ec, data["result"])
	}
	result, _ := data["result"].(map[string]any)
	stok, _ := result["stok"].(string)
	if stok == "" {
		return fmt.Errorf("login: missing stok")
	}
	c.mu.Lock()
	c.stok = stok
	hasCookie := c.sysauth != ""
	c.mu.Unlock()
	if !hasCookie {
		return fmt.Errorf("login: missing sysauth cookie")
	}
	return nil
}

func (c *Client) loginIfNeeded() error {
	c.mu.RLock()
	ok := c.seq != nil && c.stok != "" && c.sysauth != ""
	c.mu.RUnlock()
	if ok {
		return nil
	}
	return c.Login()
}

func (c *Client) encodePayload(body any) ([]byte, error) {
	data, err := c.encodeData(body)
	if err != nil {
		return nil, err
	}
	sign, err := c.encodeSign(len(data))
	if err != nil {
		return nil, err
	}
	return []byte("sign=" + sign + "&data=" + url.QueryEscape(data)), nil
}

func (c *Client) encodeData(body any) (string, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	// Compact like Python separators=(",", ":") — encoding/json already does that for maps.
	enc, err := aesEncryptCBC(c.aesKeyB, c.aesIVB, raw)
	if err != nil {
		return "", err
	}
	return b64Encode(enc), nil
}

func (c *Client) encodeSign(dataLen int) (string, error) {
	if c.seq == nil {
		return "", fmt.Errorf("seq is nil")
	}
	seqWithLen := *c.seq + dataLen
	h := authHash(c.username, c.password)
	signText := fmt.Sprintf("k=%s&i=%s&h=%s&s=%d", c.aesKey, c.aesIV, h, seqWithLen)
	return rsaEncryptTPLink(c.signRSAN, c.signRSAE, []byte(signText))
}

func (c *Client) decryptData(data string) (map[string]any, error) {
	if data == "" {
		c.mu.Lock()
		c.clearAuth()
		c.mu.Unlock()
		return nil, fmt.Errorf("empty encrypted data")
	}
	c.mu.RLock()
	keyB, ivB := append([]byte(nil), c.aesKeyB...), append([]byte(nil), c.aesIVB...)
	c.mu.RUnlock()
	decoded, err := b64Decode(data)
	if err != nil {
		return nil, err
	}
	plain, err := aesDecryptCBC(keyB, ivB, decoded)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) withRetry(fn func() error) error {
	err := fn()
	if err == nil {
		return nil
	}
	msg := err.Error()
	// Do not burn more attempts on bad credentials.
	if strings.Contains(msg, "error_code=-5002") || strings.Contains(msg, "error_code=1") {
		return err
	}
	// Re-login once on auth-ish failures (expired stok / empty data / 403).
	c.mu.Lock()
	c.clearAuth()
	c.mu.Unlock()
	if loginErr := c.Login(); loginErr != nil {
		return fmt.Errorf("%v (relogin: %w)", err, loginErr)
	}
	return fn()
}

// ListDevices returns raw Deco mesh device entries.
func (c *Client) ListDevices() ([]map[string]any, error) {
	var devices []map[string]any
	err := c.withRetry(func() error {
		if err := c.loginIfNeeded(); err != nil {
			return err
		}
		c.mu.RLock()
		stok := c.stok
		c.mu.RUnlock()
		path := fmt.Sprintf("/cgi-bin/luci/;stok=%s/admin/device", stok)
		out, err := c.post(path, url.Values{"form": {"device_list"}},
			map[string]any{"operation": "read"}, true)
		if err != nil {
			return err
		}
		dataStr, _ := out["data"].(string)
		data, err := c.decryptData(dataStr)
		if err != nil {
			return err
		}
		if ec := asInt(data["error_code"]); ec != 0 {
			return fmt.Errorf("device_list error_code=%d", ec)
		}
		result, _ := data["result"].(map[string]any)
		list, _ := result["device_list"].([]any)
		devices = make([]map[string]any, 0, len(list))
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if nick, ok := m["custom_nickname"].(string); ok {
				m["custom_nickname"] = decodeName(nick)
			}
			devices = append(devices, m)
		}
		return nil
	})
	return devices, err
}

// ListClients returns stations for one Deco (or "default" for whole mesh).
// Each client map is annotated with "_deco_mac" from the query when the API
// omits association.
func (c *Client) ListClients(decoMAC string) ([]map[string]any, error) {
	if decoMAC == "" {
		decoMAC = "default"
	}
	var clients []map[string]any
	err := c.withRetry(func() error {
		if err := c.loginIfNeeded(); err != nil {
			return err
		}
		c.mu.RLock()
		stok := c.stok
		c.mu.RUnlock()
		path := fmt.Sprintf("/cgi-bin/luci/;stok=%s/admin/client", stok)
		payload := map[string]any{
			"operation": "read",
			"params":    map[string]any{"device_mac": decoMAC},
		}
		out, err := c.post(path, url.Values{"form": {"client_list"}}, payload, true)
		if err != nil {
			return err
		}
		dataStr, _ := out["data"].(string)
		data, err := c.decryptData(dataStr)
		if err != nil {
			return err
		}
		if ec := asInt(data["error_code"]); ec != 0 {
			return fmt.Errorf("client_list error_code=%d", ec)
		}
		result, _ := data["result"].(map[string]any)
		list, _ := result["client_list"].([]any)
		clients = make([]map[string]any, 0, len(list))
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := m["name"].(string); ok {
				m["name"] = decodeName(name)
			}
			if decoMAC != "default" {
				m["_deco_mac"] = normMAC(decoMAC)
			}
			clients = append(clients, m)
		}
		return nil
	})
	return clients, err
}

// ListAllClients queries each Deco in parallel (association needs per-AP lists),
// falling back to a single global query if all per-AP calls fail.
func (c *Client) ListAllClients(meshMACs []string) ([]map[string]any, error) {
	if len(meshMACs) == 0 {
		return c.ListClients("default")
	}
	type res struct {
		list []map[string]any
		err  error
	}
	ch := make(chan res, len(meshMACs))
	for _, mac := range meshMACs {
		mac := mac
		go func() {
			list, err := c.ListClients(mac)
			ch <- res{list, err}
		}()
	}
	seen := map[string]struct{}{}
	var all []map[string]any
	var lastErr error
	okAny := false
	for range meshMACs {
		r := <-ch
		if r.err != nil {
			lastErr = r.err
			continue
		}
		okAny = true
		for _, m := range r.list {
			cmac := normMAC(strField(m, "mac", "client_mac"))
			if cmac == "" {
				continue
			}
			if _, dup := seen[cmac]; dup {
				continue
			}
			seen[cmac] = struct{}{}
			all = append(all, m)
		}
	}
	if okAny {
		return all, nil
	}
	if lastErr != nil {
		list, err := c.ListClients("default")
		if err != nil {
			return nil, lastErr
		}
		return list, nil
	}
	return all, nil
}

func decodeName(name string) string {
	if name == "" {
		return name
	}
	decoded, err := b64Decode(name)
	if err != nil {
		return name
	}
	s := string(decoded)
	if s == "" {
		return name
	}
	return s
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return 0
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
