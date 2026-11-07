# acer-swift-sfa14-11

ACPI tables from pull request #163 by deskflag-cfc to aarch64-laptops/build, closed without merging (https://github.com/aarch64-laptops/build/pull/163), at commit b0dfbce32586c2e3b1c0de320439daf614449950, folder misc/acer-swift-sfa14-11.

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

Left out: MSDM (the Windows product key).

Copied by scripts/archive.ts.
