import status from '../data/status.json';

export type Model = (typeof status.models)[number];
export type Tone = 'good' | 'close' | 'wanted' | 'muted';

export const TONE: Record<Tone, string> = {
  good: 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30',
  close: 'bg-amber-500/15 text-amber-300 ring-amber-500/30',
  wanted: 'bg-rose-500/15 text-rose-300 ring-rose-500/30',
  muted: 'bg-zinc-500/10 text-zinc-400 ring-zinc-500/20',
};

export type Cell = { label: string; tone: Tone; title: string; href?: string };

const release = (tag: string) => tag.replace(/^v/, '');
const hasUpstreamTree = (model: Model) => model.upstream.deviceTree === 'released' || model.upstream.deviceTree === 'next';

export function omarchyCell(model: Model): Cell {
  if (model.omarchy === 'works') return { label: 'Boots', tone: 'good', title: `Omarchy's kernel has the device tree and the image picks it by itself` };
  if (model.omarchy === 'manual') return { label: 'Not picked', tone: 'close', title: `Omarchy's kernel has the device tree, but no hardware IDs point the image to it` };
  if (hasUpstreamTree(model)) return { label: 'Newer kernel', tone: 'close', title: `Linux has the device tree, Omarchy's kernel ${release(status.omarchy.kernelBase)} doesn't yet` };
  return { label: 'No device tree', tone: 'wanted', title: 'Nobody has written a device tree for this model yet' };
}

export function deviceTreeCell(model: Model): Cell {
  const { deviceTree, firstRelease, devicetreePosted } = model.upstream;
  if (deviceTree === 'released') return { label: `Linux ${release(firstRelease!)}`, tone: 'good', title: `In Linux since ${release(firstRelease!)}` };
  if (deviceTree === 'next') return { label: 'linux-next', tone: 'close', title: 'Queued for the next Linux release' };
  if (deviceTree === 'posted') return { label: 'Posted', tone: 'close', title: 'Sent to the kernel mailing list, not merged yet', href: devicetreePosted ?? undefined };
  return { label: 'Wanted', tone: 'wanted', title: 'No device tree exists for this model' };
}

export function idsCell(model: Model): Cell {
  const ids = model.variants.flatMap((variant) => variant.ids).filter((entry) => entry.matches);
  if (model.upstream.ids === 'listed') {
    const first = ids.map((entry) => entry.firstRelease).filter(Boolean).sort()[0];
    return { label: first ? `systemd ${release(first)}` : 'systemd main', tone: 'good', title: 'systemd knows its hardware IDs, so a boot image can pick its device tree by itself' };
  }
  if (model.upstream.ids === 'mismatch') return { label: 'Mismatch', tone: 'close', title: `systemd's entry names a different device tree than the kernel's, so it never matches` };
  return { label: 'Wanted', tone: 'wanted', title: 'Nobody has sent its hardware IDs yet' };
}

export function firmwareCell(model: Model): Cell {
  if (model.upstream.firmware === 'linux-firmware') return { label: 'linux-firmware', tone: 'good', title: model.upstream.firmwareFolders.join(', ') };
  if (model.upstream.firmware === 'download') return { label: 'Vendor download', tone: 'close', title: `Not in linux-firmware, but the vendor's driver package has it`, href: model.upstream.firmwareDownload ?? undefined };
  if (model.upstream.firmware === 'audio') return { label: 'Audio only', tone: 'close', title: `linux-firmware has its audio topology; the signed DSP and GPU files still come from Windows` };
  return { label: 'From Windows', tone: 'muted', title: `Not in linux-firmware; today it comes from the laptop's own Windows` };
}

export function acpiCell(model: Model): Cell {
  if (model.upstream.acpi === 'collected') return { label: 'Collected', tone: 'good', title: 'Its ACPI tables are in the aarch64-laptops collection', href: model.upstream.acpiUrls[0] };
  if (hasUpstreamTree(model)) return { label: 'Not needed', tone: 'muted', title: 'It already has a device tree' };
  return { label: 'Wanted', tone: 'wanted', title: 'A device tree is written from these, so this is the first thing to send' };
}

export const GENERATION: Record<string, string> = { x1: 'Snapdragon X', x2: 'Snapdragon X2', '8cx': 'Snapdragon 8cx Gen 3', n1x: 'NVIDIA N1x', gb10: 'NVIDIA GB10' };

export type NvidiaModel = (typeof status.nvidia)[number];

export function nvidiaOmarchyCell(model: NvidiaModel): Cell {
  if (model.omarchy === 'works') return { label: 'Boots', tone: 'good', title: 'A tester booted Omarchy on it' };
  if (model.omarchy === 'expected') return { label: 'Should boot', tone: 'close', title: 'Same platform as a tested model, but nobody has tried it yet' };
  if (model.omarchy === 'untested') return { label: 'Not tested', tone: 'muted', title: 'Nobody has tried Omarchy on it yet' };
  if (model.omarchy === 'problems') return { label: 'Problems', tone: 'wanted', title: 'A tester booted it and hit problems' };
  return { label: 'Not yet', tone: 'wanted', title: `Omarchy doesn't support it yet` };
}

export function nvidiaLinuxCell(model: NvidiaModel): Cell {
  if (model.linux === 'mainline') return { label: 'Mainline', tone: 'good', title: 'Plain Linux supports it' };
  return { label: `Omarchy's kernel`, tone: 'close', title: `Needs patches that Omarchy carries in linux-omarchy and that aren't in Linux yet` };
}
