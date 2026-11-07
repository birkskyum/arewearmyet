# microsoft-surface-romulus13_and_15

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/microsoft-surface-romulus13_and_15 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/microsoft-surface-romulus13_and_15).

- apic.dat (APIC.aml)
- bgrt.dat (BGRT.aml)
- csrt.dat (CSRT.aml)
- dbg2.dat (DBG2.aml)
- dsdt.dat (DSDT.aml)
- facp.dat (FACP.aml)
- facs.dat (FACS.aml)
- gtdt.dat (GTDT.aml)
- iort.dat (IORT.aml)
- mcfg.dat (MCFG.aml)
- pptt.dat (PPTT.aml)
- sdev.dat (SDEV.aml)
- spcr.dat (SPCR.aml)
- tpm2.dat (TPM2.aml)
- SSDT.dsl (SSDT.dsl, disassembled)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
