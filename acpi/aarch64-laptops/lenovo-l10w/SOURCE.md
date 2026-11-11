# lenovo-l10w

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/lenovo-l10w (https://github.com/aarch64-laptops/build/tree/HEAD/misc/lenovo-l10w).

- apic.dat (acpi/APIC.aml)
- bgrt.dat (acpi/BGRT.aml)
- csrt.dat (acpi/CSRT.aml)
- dbg2.dat (acpi/DBG2.aml)
- dsdt.dat (acpi/DSDT.aml)
- facp.dat (acpi/FACP.aml)
- fpdt.dat (acpi/FPDT.aml)
- gtdt.dat (acpi/GTDT.aml)
- iort.dat (acpi/IORT.aml)
- mcfg.dat (acpi/MCFG.aml)
- pptt.dat (acpi/PPTT.aml)
- spcr.dat (acpi/SPCR.aml)
- tpm2.dat (acpi/TPM2.aml)
- xsdt.dat (acpi/XSDT.aml)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: MSDM (the Windows product key).

Copied by scripts/archive.ts.
