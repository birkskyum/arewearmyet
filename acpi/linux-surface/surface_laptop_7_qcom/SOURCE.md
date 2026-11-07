# surface_laptop_7_qcom

ACPI tables from linux-surface/acpidumps at commit 1d0a2ce742b450fe3f65287adbe174ddccabe228, folder surface_laptop_7_qcom (https://github.com/linux-surface/acpidumps/tree/HEAD/surface_laptop_7_qcom).

- mcfg.dat (dump.txt)
- facp.dat (dump.txt)
- apic.dat (dump.txt)
- pptt.dat (dump.txt)
- gtdt.dat (dump.txt)
- fpdt.dat (dump.txt)
- ssdt.dat (dump.txt)
- bgrt.dat (dump.txt)
- bert.dat (dump.txt)
- csrt.dat (dump.txt)
- dbg2.dat (dump.txt)
- iort.dat (dump.txt)
- sdev.dat (dump.txt)
- spcr.dat (dump.txt)
- tpm2.dat (dump.txt)
- xsdt.dat (dump.txt)
- dsdt.dat (dump.txt)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: MSDM (the Windows product key).

Copied by scripts/archive.ts.
