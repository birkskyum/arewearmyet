# asus-tp370ql

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/asus-tp370ql (https://github.com/aarch64-laptops/build/tree/HEAD/misc/asus-tp370ql).

- acpi-asus-tp370ql-dsdt.dat (acpi-asus-tp370ql-dsdt.dat)
- acpi-asus-tp370ql-facp.dat (acpi-asus-tp370ql-facp.dat)
- acpi-asus-tp370ql-facs.dat (acpi-asus-tp370ql-facs.dat)
- acpi-asus-tp370ql-xsdt.dat (acpi-asus-tp370ql-xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
