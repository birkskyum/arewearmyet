# samsung-galaxy-book-go-5g

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/samsung-galaxy-book-go-5g (https://github.com/aarch64-laptops/build/tree/HEAD/misc/samsung-galaxy-book-go-5g).

- apic.dat (APIC.dat)
- bgrt.dat (BGRT.dat)
- csrt.dat (CSRT.dat)
- dbg2.dat (DBG2.dat)
- dsdt.dat (DSDT.dat)
- facp.dat (FACP.dat)
- facs.dat (FACS.dat)
- fpdt.dat (FPDT.dat)
- gtdt.dat (GTDT.dat)
- iort.dat (IORT.dat)
- mcfg.dat (MCFG.dat)
- pptt.dat (PPTT.dat)
- tpm2.dat (TPM2.dat)
- xsdt.dat (XSDT.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
