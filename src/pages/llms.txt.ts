// The usual entry point for agents, pointing at the guide and the API.
export function GET() {
  const text = `# Are we ARM yet?

> Which ARM PCs (Snapdragon X, X2, 8cx, 7c, C, NVIDIA N1x and GB10, Googlebooks, Chromebooks) run Linux and Omarchy, model by model, and what works on each past booting.

Agents helping an owner check their machine, report what works or propose fixes to Omarchy: read the guide first.

- [Guide for AI agents](https://arewearmyet.com/agents.md): the rules, how to measure a computer with hw-test, report results, try a fix from a boot entry of its own, show what it changed and open a pull request on Omarchy
- [Checks](https://arewearmyet.com/api/checks): what each check means, how it's measured and where to look when it fails
- [Notes](https://arewearmyet.com/api/notes): problems, workarounds and pointers owners and their agents left, with logs. Add ?model=<id> or ?q=<words>
- [Probe script](https://arewearmyet.com/probe.sh): the read-only quick check for distros other than Omarchy
- [Computer models](https://arewearmyet.com/api/models): every laptop, tablet and desktop model with its page and API link
- [Firmware sources](https://arewearmyet.com/firmware-sources.json): where each model's signed firmware can be downloaded
`;
  return new Response(text, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
}
