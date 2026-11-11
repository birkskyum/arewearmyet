# lenovo-yoga-c630

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/lenovo-yoga-c630 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/lenovo-yoga-c630).

- apic.dat (acpi/APIC.bin)
- bgrt.dat (acpi/BGRT.bin)
- csrt.dat (acpi/CSRT.bin)
- dbg2.dat (acpi/DBG2.bin)
- dsdt.dat (acpi/DSDT.bin)
- facp.dat (acpi/FACP.bin)
- facs.dat (acpi/FACS.bin)
- fpdt.dat (acpi/FPDT.bin)
- gtdt.dat (acpi/GTDT.bin)
- iort.dat (acpi/IORT.bin)
- mcfg.dat (acpi/MCFG.bin)
- pptt.dat (acpi/PPTT.bin)
- ssdt.dat (acpi/SSDT.bin)
- tpm2.dat (acpi/TPM2.bin)
- xsdt.dat (acpi/XSDT.bin)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
