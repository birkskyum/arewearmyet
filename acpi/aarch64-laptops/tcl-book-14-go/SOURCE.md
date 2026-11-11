# tcl-book-14-go

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/tcl-book-14-go (https://github.com/aarch64-laptops/build/tree/HEAD/misc/tcl-book-14-go).

- apic.dat (apic.dat)
- bgrt.dat (bgrt.dat)
- csrt.dat (csrt.dat)
- dbg2.dat (dbg2.dat)
- dsdt.dat (dsdt.dat)
- facp.dat (facp.dat)
- fpdt.dat (fpdt.dat)
- gtdt.dat (gtdt.dat)
- iort.dat (iort.dat)
- mcfg.dat (mcfg.dat)
- pptt.dat (pptt.dat)
- spcr.dat (spcr.dat)
- tpm2.dat (tpm2.dat)
- xsdt.dat (xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
