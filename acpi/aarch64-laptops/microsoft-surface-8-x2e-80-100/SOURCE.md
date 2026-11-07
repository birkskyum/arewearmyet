# microsoft-surface-8-x2e-80-100

ACPI tables from pull request #176 by ggarmidi to aarch64-laptops/build, not merged yet (https://github.com/aarch64-laptops/build/pull/176), at commit d7ae771f520e6179f9aae01969b291eae450871b, folder misc/microsoft-surface-8-x2e-80-100.

- apic.dat (apic.dat)
- bert.dat (bert.dat)
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
- sdev.dat (sdev.dat)
- spcr.dat (spcr.dat)
- ssdt.dat (ssdt.dat)
- tpm2.dat (tpm2.dat)
- xsdt.dat (xsdt.dat)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Left out: MSDM (the Windows product key), RGRT (the regulatory logo, an image).

Copied by scripts/archive.ts.
