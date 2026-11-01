# ASUS Ascent QN10

ACPI tables dumped by Sonicadvance1 (HdkR) and sent to aarch64-laptops in https://github.com/aarch64-laptops/build/pull/175, not merged yet. These are the binary tables from that pull request's commit e17341e4c9fce07b6acda13b1b4bd8da80ac8cba (misc/asus-qn10-16e/acpi). Disassemble with `iasl -d dsdt.dat`.

The DSDT's SOSN, the chip's serial number, is zeroed here and the table's checksum fixed. Once the pull request is merged, the board reads the tables from aarch64-laptops and this copy can go.
