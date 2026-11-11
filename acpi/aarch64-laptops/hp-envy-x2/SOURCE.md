# hp-envy-x2

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/hp-envy-x2 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/hp-envy-x2).

- acpi-hp-envy-x2-dsdt.dat (acpi-hp-envy-x2-dsdt.dat)
- acpi-hp-envy-x2-facp.dat (acpi-hp-envy-x2-facp.dat)
- acpi-hp-envy-x2-facs.dat (acpi-hp-envy-x2-facs.dat)
- acpi-hp-envy-x2-ssdt.dat (acpi-hp-envy-x2-ssdt.dat)
- acpi-hp-envy-x2-ssdt1.dat (acpi-hp-envy-x2-ssdt1.dat)
- acpi-hp-envy-x2-ssdt2.dat (acpi-hp-envy-x2-ssdt2.dat)
- acpi-hp-envy-x2-ssdt3.dat (acpi-hp-envy-x2-ssdt3.dat)
- acpi-hp-envy-x2-xsdt.dat (acpi-hp-envy-x2-xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
