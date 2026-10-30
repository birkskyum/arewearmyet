# Dell XPS Googlebook (Mica) device tree

`x1e80100-google-mica.dts` is the experimental mainline-style device tree that was used to boot
Linux from an external USB drive on a Dell XPS Googlebook (Mica, Snapdragon X1E80100). It is a
reference for the board, not a finished or upstream-ready tree.

## Where it comes from

- It is adapted from the HP Googlebook 14 (Quartz) tree in ericman2001/hp-googlebook-linux at
  [`8ed394a`](https://github.com/ericman2001/hp-googlebook-linux/tree/8ed394a30c0fdc8a012b3a1131295ce18b48d0fe/gentoo/etc/portage/patches/sys-kernel/gentoo-kernel),
  as it stands after all 22 patches there are applied to Linux 7.2.11.
- The Dell-specific wiring was taken from the device tree the stock Googlebook OS boots with on
  this laptop. That covers the keyboard keymap, touchpad (reset on GPIO 2, L12B 1.2 V supply),
  touchscreen (I2C address 0x10, HID descriptor at 0, L15B supply) and backlight (PMK8550 PWM0
  on GPIO 5, enabled through PMC8380 GPIO 10 and GPIO 4).
- HP-only wiring (Quartz panel, backlight, GPIO and USB connector layout) was not carried over.
  Mica and Quartz share the platform but are wired differently. The HP tree is not expected to
  work unmodified on the Dell.
- The file is copied byte for byte from the one that was compiled and booted. Its SHA-256 is
  `caada5d47c7038061c836a25c49e25eaa6f802d8d31948f8c9254bbc93997573`. Comments from the
  generator were not kept, and the SPDX line appears twice. That is cosmetic, and it is left in
  so the hash still matches.

## Licence

BSD-3-Clause, the same as the HP device tree it is derived from (see that repository's README).
The stock Google device trees are not included here (see below), because their redistribution
terms aren't established.

## What it needs

- It builds against the device tree includes in Linux 7.2.11 (`hamoa.dtsi`,
  `hamoa-pmics.dtsi`). The HP patches don't change those files, but the kernel needs the HP
  driver patches to boot on these machines. Those cover the SPI chip-select fix, the eUSB2
  repeater reset, the eDP panel enable GPIO, the SMMU identity domain, the remoteproc domain
  fallback, ChromeOS EC over USB, cros-ec GPIO probing and the PCIe GDSC retention changes.
- The laptop boots it through Depthcharge as an Android boot image v4, at EL2, with the tree in
  `dtbo`. It does not use UEFI or a UKI.
- The ADSP node names `qcom/x1e80100/google/mica/adsp.mdt` and `adsp_dtb.mdt`. That firmware is
  signed by the vendor and is not part of this repository.
- `google,mica` isn't a documented compatible in Linux, so `dtbs_check` will flag it.

## What it leaves out on purpose

- Internal NVMe is turned off (`pcie6a`, `pcie6a_phy` and `vreg_nvme` are disabled), so the
  experiments could not touch the internal disk. Don't expect the internal drive with this tree.
- The two USB-C ports are set up through their USB 2.0 PHYs first.
- As in the HP tree, `gpu_zap_shader`, `gic_its` and `sbsa_watchdog` are disabled.

## What has been checked

- Recompiled with `cpp` and `dtc -@` against the Linux 7.2.11 includes, it gives a DTB with
  SHA-256 `c8fbf1b0d7169d6886a15b8975a73123d9877eca52dfb1de9834ba6b1345fcaf`. That is the same
  DTB that was packed, LZ4-compressed, as entry 0 of the `dtbo` image the USB test builds booted.
  `dtc` gives 30 `unique_unit_address` warnings, all on the shared QUP i2c/spi/serial nodes in
  `hamoa.dtsi` and none in this file.
- On the laptop, an earlier build that started the ADSP early ran for a 120 s observation with
  the USB root filesystem mounted read-only.
- In the latest build the ADSP and PMIC clients come up, then the GPU and display modules load,
  and MSM DRM and the GPU firmware start. The panel shows up as an unknown `LGD 0x0804`, so
  conservative timings are used. Waiting for graphics then times out after about 54 s, before
  systemd hands off to the desktop. A display state and probe ordering problem is the leading
  guess, but it isn't confirmed.

None of this shows a working desktop, charging, audio, Wi-Fi, Bluetooth, suspend, an install to
the internal drive, dual boot, or long-term reliability. The Wi-Fi, Bluetooth and DisplayPort
nodes come from the HP tree and haven't been confirmed on the Dell.

## The stock tree, for comparison

The stock trees can be extracted from Google's official recovery image. They aren't copied here.

- Recovery image: https://dl.google.com/device/recovery/mica-user/16560795/recovery.zip
- Listed in https://dl.google.com/dl/edgedl/device/recovery/production_recovery.json as build
  `16560795_CL3B.260622.276.R2` (the inventory was dated 2026-10-22), with archive MD5
  `be97fb4fe368e50bb6102dcede1b2a80`. The MD5 only catches a corrupt download. It doesn't
  authenticate the image.

The archive expands to a disk image of about 19 GB with a GPT. In that build, `dtbo_a` starts at
512-byte sector 25972736 and is 16384 sectors long (8 MiB). Its SHA-256 is
`692e82ffc2672ee3b47ea99d411db4e6c039937d356bbf4e596928ce5c3f8e07`. Check the offsets with
`sfdisk --json` on the image you have, then:

```sh
dd if=<disk image> of=dtbo_a.img bs=512 skip=25972736 count=16384
```

`dtbo_a.img` is an Android DT table (magic `d7b7ab1e`, version 1) with three LZ4-compressed
entries: the base tree, the Mica board overlay and a revision overlay. This splits it into
`entry-0.dtb` to `entry-2.dtb`:

```sh
#!/bin/sh
set -eu
img=$1
be32() { od -A n -t u1 -j "$2" -N 4 "$1" | awk '{print ($1*16777216)+($2*65536)+($3*256)+$4}'; }
[ "$(od -A n -t x1 -N 4 "$img" | tr -d ' ')" = d7b7ab1e ] || { echo "not a DT table" >&2; exit 1; }
esize=$(be32 "$img" 12); count=$(be32 "$img" 16); first=$(be32 "$img" 20)
i=0
while [ "$i" -lt "$count" ]; do
  e=$((first + i * esize))
  size=$(be32 "$img" "$e"); off=$(be32 "$img" $((e + 4)))
  dd if="$img" of="entry-$i.raw" bs=1 skip="$off" count="$size" status=none
  if [ "$(od -A n -t x1 -N 4 "entry-$i.raw" | tr -d ' ')" = 04224d18 ]; then
    lz4 -dc "entry-$i.raw" > "entry-$i.dtb"; rm "entry-$i.raw"
  else
    mv "entry-$i.raw" "entry-$i.dtb"
  fi
  i=$((i + 1))
done
```

For that build, the SHA-256 sums of the three entries should be:

```
4d019aa922b3d92695bafbc741bd3fd0073454995410a43d98904f4f9dac0f84  entry-0.dtb  (base, 414268 bytes)
ae86b73044c9b6bc2af8318291c62d2cbef53bdbe4eea09cbaecafeccb375e84  entry-1.dtb  (Mica board overlay, 36860 bytes)
678a35b9f776a8aa759e542b5f31dc0a2308e940e5d2e3985e69675de9b1c4fd  entry-2.dtb  (revision overlay, 996 bytes)
```

`dtc -I dtb -O dts entry-1.dtb` decompiles the board overlay. The tree the running stock kernel
exposes in `/sys/firmware/devicetree` also has values filled in at boot, such as the KASLR seed,
initrd addresses and the kernel command line, so check it before sharing it.

## Redactions

Nothing in `x1e80100-google-mica.dts` needed removing. It was scanned for serial numbers, MAC
addresses, UUIDs, boot seeds, KASLR or RNG seeds and local paths. The only match was the
`serial1 = &uart14` alias. Firmware, logs, recovery and partition images, and the decompiled
stock and runtime trees were left out.

## Board wiring

To show it on the board, the `dell-xps-googlebook` entry in `data/models.yaml` can list
`data/device-trees/dell-xps-googlebook/x1e80100-google-mica.dts` under `devicetree_files`.
This change leaves `data/models.yaml` alone.
