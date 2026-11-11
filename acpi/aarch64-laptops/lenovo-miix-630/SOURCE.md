# lenovo-miix-630

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/lenovo-miix-630 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/lenovo-miix-630).

- dsdt.dat (dsdt.dat)
- facp.dat (facp.dat)
- facs.dat (facs.dat)
- ssdt.dat (ssdt.dat)
- ssdt1.dat (ssdt1.dat)
- xsdt.dat (xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
