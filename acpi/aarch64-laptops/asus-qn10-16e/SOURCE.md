# asus-qn10-16e

ACPI tables from pull request #175 by Sonicadvance1 to aarch64-laptops/build, not merged yet (https://github.com/aarch64-laptops/build/pull/175), at commit e17341e4c9fce07b6acda13b1b4bd8da80ac8cba, folder misc/asus-qn10-16e.

- apic.dat (acpi/apic.dat)
- bert.dat (acpi/bert.dat)
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
- spcr.dat (acpi/spcr.dat)
- tpm2.dat (acpi/tpm2.dat)
- uefi.dat (acpi/uefi.dat)
- xsdt.dat (acpi/xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
