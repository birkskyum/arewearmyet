# asus-vivobook-s-15

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/asus-vivobook-s-15 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/asus-vivobook-s-15).

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

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
