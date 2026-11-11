# surface_pro_x_sq1

ACPI tables from linux-surface/acpidumps at commit 1d0a2ce742b450fe3f65287adbe174ddccabe228, folder surface_pro_x_sq1 (https://github.com/linux-surface/acpidumps/tree/HEAD/surface_pro_x_sq1).

- mcfg.dat (acpidump.out)
- gtdt.dat (acpidump.out)
- apic.dat (acpidump.out)
- iort.dat (acpidump.out)
- tpm2.dat (acpidump.out)
- bgrt1.dat (acpidump.out)
- dsdt.dat (acpidump.out)
- dbg2.dat (acpidump.out)
- bert.dat (acpidump.out)
- facp.dat (acpidump.out)
- bgrt2.dat (acpidump.out)
- fpdt.dat (acpidump.out)
- csrt.dat (acpidump.out)
- pptt.dat (acpidump.out)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
