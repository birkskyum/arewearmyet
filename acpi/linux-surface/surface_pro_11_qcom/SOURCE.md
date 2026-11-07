# surface_pro_11_qcom

ACPI tables from linux-surface/acpidumps at commit 1d0a2ce742b450fe3f65287adbe174ddccabe228, folder surface_pro_11_qcom (https://github.com/linux-surface/acpidumps/tree/HEAD/surface_pro_11_qcom).

- mcfg.dat (acpidump.txt)
- facp.dat (acpidump.txt)
- apic.dat (acpidump.txt)
- pptt.dat (acpidump.txt)
- gtdt.dat (acpidump.txt)
- fpdt.dat (acpidump.txt)
- ssdt.dat (acpidump.txt)
- bgrt.dat (acpidump.txt)
- csrt.dat (acpidump.txt)
- dbg2.dat (acpidump.txt)
- iort.dat (acpidump.txt)
- sdev.dat (acpidump.txt)
- spcr.dat (acpidump.txt)
- tpm2.dat (acpidump.txt)
- xsdt.dat (acpidump.txt)
- dsdt.dat (acpidump.txt)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: RGRT (the regulatory logo, an image).

Copied by scripts/archive.ts.
