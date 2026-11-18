// The usual entry point for agents, pointing at the guide and the API.
export function GET() {
  const text = `# Are we ARM yet?

> Which ARM PCs (Snapdragon X, X2, 8cx, 7c, C, NVIDIA N1x and GB10, Googlebooks, Chromebooks) run Linux and Omarchy, model by model, and what works on each past booting.

Agents helping an owner check their machine, report what works or propose fixes to Omarchy: read the guide first.

- [Guide for AI agents](https://arewearmyet.com/agents.md): rules, how to find the model, run the probe, report results and propose fixes to Omarchy
- [Probe script](https://arewearmyet.com/probe.sh): read-only check of a Snapdragon PC under Linux
- [Checks](https://arewearmyet.com/api/checks): what each check means and how it's checked
- [Models](https://arewearmyet.com/api/models): every model with its page and API link
- [Firmware sources](https://arewearmyet.com/firmware-sources.json): where each model's signed firmware can be downloaded
`;
  return new Response(text, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
}
