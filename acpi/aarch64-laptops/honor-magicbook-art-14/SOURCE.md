# honor-magicbook-art-14

ACPI tables from aarch64-laptops/build at commit 2e58842f5fa2f87771c2df017ae4d8c65225ef10, folder misc/honor-magicbook-art-14 (https://github.com/aarch64-laptops/build/tree/HEAD/misc/honor-magicbook-art-14).

- apic.dat (acpi/APIC.11)
- bgrt.dat (acpi/BGRT.19)
- csrt.dat (acpi/CSRT.7)
- dbg2.dat (acpi/DBG2.8)
- dsdt.dat (acpi/DSDT.2)
- facp.dat (acpi/FACP.1)
- fft_.dat (acpi/FFT_.4)
- fpdt.dat (acpi/FPDT.6)
- gtdt.dat (acpi/GTDT.9)
- iort.dat (acpi/IORT.10)
- mcfg.dat (acpi/MCFG.12)
- pptt.dat (acpi/PPTT.13)
- sdev.dat (acpi/SDEV.14)
- spcr.dat (acpi/SPCR.15)
- ssdt.dat (acpi/SSDT.5)
- tpm2.dat (acpi/TPM2.16)
- uefi.dat (acpi/UEFI.3)
- xsdt.dat (acpi/XSDT.0)

Disassemble with `iasl -d dsdt.dat`.

The SOSN, the chip's serial number, is zeroed in 1 table, with each binary table's checksum fixed.

Copied by scripts/archive.ts.
