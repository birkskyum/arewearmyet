# Microsoft Surface Laptop 13-inch (2nd Edition)

ACPI tables taken out of Microsoft's driver and firmware MSI ([download](https://download.microsoft.com/download/30ff24b5-e65e-4fde-9c22-491e6ac81c16/SurfaceLaptop13in2ndEditionwithSnapdragon_Win11_28000_26.083.37733.0.msi), sha256 `6301570ca49f810c6b63b0eb5c2f729bb3072508e42cdfa980be65af8f1950ef`), from the UEFI image Surface_UEFI_8.5175.155.bin inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-dea80d45.dat`
- `BERT-QCOMEDK2-c2f9d2d8.dat`
- `BGRT-QCOMEDK2-9fd0386c.dat`
- `CSRT-QCOMEDK2-720cc408.dat`
- `DBG2-QCOMEDK2-0f91e64d.dat`
- `DSDT-SDM8480-0395a87c.dat`
- `FACP-QCOMEDK2-6a407175.dat`
- `GTDT-QCOMEDK2-24dbe14d.dat`
- `IORT-QCOMEDK2-d4f3eed6.dat`
- `MCFG-QCOMEDK2-f12de3ad.dat`
- `PPTT-QCOMEDK2-15551956.dat`
- `SPCR-QCOMEDK2-cff96fa1.dat`
- `SSDT-SsdtManf-8a94e491.dat`
- `SSDT-SsdtSurf-06f53eab.dat`
- `TPM2-QCOMEDK2-bccc0db1.dat`
