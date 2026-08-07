<script lang="ts">
  interface Props {
    source: string | null | undefined;
    connected?: boolean;
  }
  let { source, connected = true }: Props = $props();

  const label = $derived.by(() => {
    if (!connected) return 'disconnected';
    if (!source) return 'unknown';
    if (source === 'esp32') return 'esp32';
    if (source === 'simulated' || source === 'simulate') return 'simulated';
    return source;
  });

  const cls = $derived.by(() => {
    if (!connected) return 'bad';
    if (label === 'esp32') return 'ok';
    if (label === 'simulated') return 'warn';
    return 'muted';
  });
</script>

<span class={`chip ${cls}`}>src {label}</span>
