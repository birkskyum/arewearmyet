# asus-zenbook-14-ux3480qa

ACPI tables from pull request #178 by emirb to aarch64-laptops/build, not merged yet (https://github.com/aarch64-laptops/build/pull/178), at commit c93f6b670047df2859f3d1b1a9d3ff92f75b5972, folder misc/asus-zenbook-14-ux3480qa.

- apic.dat (acpi/apic.dat)
- bgrt.dat (acpi/bgrt.dat)
- csrt.dat (acpi/csrt.dat)
- dbg2.dat (acpi/dbg2.dat)
- dsdt.dat (acpi/dsdt.dat)
- facp.dat (acpi/facp.dat)
- fpdt.dat (acpi/fpdt.dat)
- gtdt.dat (acpi/gtdt.dat)
- iort.dat (acpi/iort.dat)
- mcfg.dat (acpi/mcfg.dat)
- pptt.dat (acpi/pptt.dat)
- sdev.dat (acpi/sdev.dat)
- spcr.dat (acpi/spcr.dat)
- tpm2.dat (acpi/tpm2.dat)
- uefi.dat (acpi/uefi.dat)
- xsdt.dat (acpi/xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
