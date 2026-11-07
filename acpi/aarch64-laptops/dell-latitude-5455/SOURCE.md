# dell-latitude-5455

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/dell-latitude-5455 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/dell-latitude-5455).

- apic.dat (apic.dat)
- bgrt.dat (bgrt.dat)
- csrt.dat (csrt.dat)
- dbg2.dat (dbg2.dat)
- dsdt.dat (dsdt.dat)
- facp.dat (facp.dat)
- fpdt1.dat (fpdt1.dat)
- gtdt.dat (gtdt.dat)
- iort.dat (iort.dat)
- mcfg.dat (mcfg.dat)
- pptt.dat (pptt.dat)
- sdev.dat (sdev.dat)
- spcr.dat (spcr.dat)
- tpm2.dat (tpm2.dat)
- xsdt.dat (xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
