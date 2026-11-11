# lenovo-ideapad-5g-14q8x05

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/lenovo-ideapad-5g-14q8x05 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/lenovo-ideapad-5g-14q8x05).

- apic.dat (APIC.aml)
- bgrt.dat (BGRT.aml)
- csrt.dat (CSRT.aml)
- dbg2.dat (DBG2.aml)
- dsdt.dat (DSDT.aml)
- facp.dat (FACP.aml)
- fpdt.dat (FPDT.aml)
- gtdt.dat (GTDT.aml)
- iort.dat (IORT.aml)
- mcfg.dat (MCFG.aml)
- pptt.dat (PPTT.aml)
- ssdt.dat (SSDT.aml)
- tpm2.dat (TPM2.aml)
- xsdt.dat (XSDT.aml)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: MSDM (the Windows product key).

Copied by scripts/archive.ts.
