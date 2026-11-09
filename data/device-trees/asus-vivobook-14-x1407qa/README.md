# ASUS Vivobook 14 (X1407QA) device trees from owners

The board's tree for this model is the one Omarchy's kernel builds, patch `2801` in omarchy-pkgs. This folder
keeps what owners sent in beside it, as evidence for what that tree still lacks.

## owner-deseret-engineer/

Sent in by deseret-engineer (issue #8, 2026-11-09), who runs Omarchy on kernel 7.3.0-3.1-omarchy with this tree.

- `running-tree.dts`: the tree the laptop runs, read back with `dtc -I fs -O dts /sys/firmware/devicetree/base`.
  It started from Linux's Zenbook A14 LCD tree and keeps that tree's `model` and `compatible`.
- `CHANGES.md`: the owner's account of every change from the Zenbook A14 LCD tree, the kernel command line, and
  what works. One mention of another owner's repository is reworded; the rest is as sent.

What it shows:

- The keyboard is HID over I2C at 0x3a on `i2c5`, with a PTN3222 eUSB2 repeater at 0x43 on the same bus. Omarchy's
  tree has both there already, from the Vivobook 16's.
- No touchscreen.
- Video decode works with `qcvss8380_pa.mbn` from the Windows driver package.
- Both cameras work: an OV02C10 on CCI bus 1 and a Himax HM1092 infrared sensor on CCI bus 0, the second with a
  driver that isn't in Linux.
- Wi-Fi (WCN6855, subsystem 105b:e130) has no board data in linux-firmware and runs on the generic NFA765 file.
- Suspend only works as s2idle. Deep suspend restarts the laptop.
- Only the left speaker plays. The right amplifier reports a fault at every stream start.
