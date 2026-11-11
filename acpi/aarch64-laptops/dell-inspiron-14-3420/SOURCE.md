# dell-inspiron-14-3420

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/dell-inspiron-14-3420 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/dell-inspiron-14-3420).

- apic.dat (acpi/APIC.dat)
- bgrt.dat (acpi/BGRT.dat)
- csrt.dat (acpi/CSRT.dat)
- dbg2.dat (acpi/DBG2.dat)
- dsdt.dat (acpi/DSDT.dat)
- facp.dat (acpi/FACP.dat)
- fpdt.dat (acpi/FPDT.dat)
- gtdt.dat (acpi/GTDT.dat)
- iort.dat (acpi/IORT.dat)
- mcfg.dat (acpi/MCFG.dat)
- pptt.dat (acpi/PPTT.dat)
- tpm2.dat (acpi/TPM2.dat)
- xsdt.dat (acpi/XSDT.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
