# acer-gateway-gwtn133-1

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/acer-gateway-gwtn133-1 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/acer-gateway-gwtn133-1).

- bgrt.dat (acpi/bgrt.dat)
- csrt.dat (acpi/csrt.dat)
- dsdt.dat (acpi/dsdt.dat)
- facp.dat (acpi/facp.dat)
- fpdt.dat (acpi/fpdt.dat)
- ssdt.dat (acpi/ssdt.dat)
- ssdt1.dat (acpi/ssdt1.dat)
- ssdt2.dat (acpi/ssdt2.dat)
- xsdt.dat (acpi/xsdt.dat)
- apic.dsl (acpi/apic.dsl, disassembled)
- dbg2.dsl (acpi/dbg2.dsl, disassembled)
- gtdt.dsl (acpi/gtdt.dsl, disassembled)
- iort.dsl (acpi/iort.dsl, disassembled)
- mcfg.dsl (acpi/mcfg.dsl, disassembled)
- pptt.dsl (acpi/pptt.dsl, disassembled)
- tpm2.dsl (acpi/tpm2.dsl, disassembled)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: MSDM (the Windows product key).

Copied by scripts/archive.ts.
