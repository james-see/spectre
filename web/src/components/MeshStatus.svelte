<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Health, type NodeRow } from '../lib/api';
  import SourceChip from './SourceChip.svelte';

  let health = $state<Health | null>(null);
  let nodes = $state<NodeRow[]>([]);
  let error = $state<string | null>(null);
  let connected = $state(false);

  async function refresh() {
    try {
      const [h, n] = await Promise.all([api.health(), api.nodes()]);
      health = h;
      nodes = n.nodes || [];
      connected = true;
      error = null;
    } catch (e) {
      connected = false;
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 500);
    return () => clearInterval(id);
  });
</script>

<div class="row" style="margin-bottom: 1rem; gap: 0.75rem;">
  <SourceChip source={health?.source} {connected} />
  {#if health}
    <span class="chip muted">tick {health.tick ?? '—'}</span>
    <span class="chip muted">clients {health.clients ?? 0}</span>
  {/if}
</div>

{#if error}
  <div class="card empty">
    Sensing-server unreachable. Start it with <code class="mono">./scripts/run_local.sh</code> then
    keep this page open. ({error})
  </div>
{:else}
  <div class="grid grid-2" style="margin-bottom: 1rem;">
    <div class="card">
      <h2>Mesh</h2>
      <div class="metric">{nodes.length}</div>
      <div class="metric-label">active nodes (expect 3)</div>
    </div>
    <div class="card">
      <h2>Health</h2>
      <div class="metric">{health?.status ?? '—'}</div>
      <div class="metric-label">HTTP /health</div>
    </div>
  </div>

  <div class="card">
    <h2>Nodes</h2>
    {#if nodes.length === 0}
      <p class="empty">No nodes reporting. Check flash/provision and UDP :5005.</p>
    {:else}
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>RSSI</th>
            <th>Last seen</th>
            <th>Motion</th>
            <th>Persons</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {#each nodes as n}
            <tr>
              <td class="mono">{n.node_id}</td>
              <td class="mono">{n.rssi_dbm?.toFixed?.(1) ?? n.rssi_dbm} dBm</td>
              <td class="mono">{n.last_seen_ms ?? '—'} ms</td>
              <td>{n.motion_level ?? '—'}</td>
              <td class="mono">{n.person_count ?? '—'}</td>
              <td>{n.status ?? '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
{/if}
