# samsung-galaxybook2

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/samsung-galaxybook2 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/samsung-galaxybook2).

- dsdt.dat (acpi/dsdt.dat)
- facp.dat (acpi/facp.dat)
- facs.dat (acpi/facs.dat)
- xsdt.dat (acpi/xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
