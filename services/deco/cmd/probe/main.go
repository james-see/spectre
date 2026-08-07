package main
import (
  "fmt"; "os"; "time"
  "github.com/joho/godotenv"
  "github.com/james-see/spectre/services/deco/internal/deco"
)
func main() {
  _ = godotenv.Overload("/Users/jc/p/spectre/.env")
  c := deco.NewClient(os.Getenv("DECO_HOST"), os.Getenv("DECO_USER"), os.Getenv("DECO_PASS"))
  t0 := time.Now()
  if err := c.Login(); err != nil { fmt.Println(err); os.Exit(1) }
  devs,_ := c.ListDevices()
  mesh := deco.NormalizeMesh(devs)
  macs := []string{}
  for _, m := range mesh { macs = append(macs, m.MAC) }
  t1 := time.Now()
  all, err := c.ListAllClients(macs)
  fmt.Println("parallel clients", len(all), err, time.Since(t1))
  fmt.Println("total", time.Since(t0))
}
