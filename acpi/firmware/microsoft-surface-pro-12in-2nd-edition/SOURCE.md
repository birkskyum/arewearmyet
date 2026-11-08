# Microsoft Surface Pro 12-inch (2nd Edition)

ACPI tables taken out of Microsoft's driver and firmware MSI ([download](https://download.microsoft.com/download/3df31041-667b-4bf4-b99e-a2722059ab53/SurfacePro12in2ndEditionwithSnapdragon_Win11_28000_26.083.37733.0.msi), sha256 `2f39eb38825a355499a45ba0e186a35d22b3bfa6309d11a5f564e7f7a893edb4`), from the UEFI image Surface_UEFI_8.5175.155.bin inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-dea80d45.dat`
- `BERT-QCOMEDK2-c2f9d2d8.dat`
- `BGRT-QCOMEDK2-9fd0386c.dat`
- `CSRT-QCOMEDK2-720cc408.dat`
- `DBG2-QCOMEDK2-0f91e64d.dat`
- `DSDT-SDM8480-55e15e48.dat`
- `FACP-QCOMEDK2-3f3c17cc.dat`
- `GTDT-QCOMEDK2-24dbe14d.dat`
- `IORT-QCOMEDK2-d4f3eed6.dat`
- `MCFG-QCOMEDK2-f12de3ad.dat`
- `PPTT-QCOMEDK2-15551956.dat`
- `SDEV-QCOMEDK2-d4b255a7.dat`
- `SPCR-QCOMEDK2-cff96fa1.dat`
- `SSDT-SsdtManf-f96f2127.dat`
- `SSDT-SsdtSurf-0daf31d0.dat`
- `TPM2-QCOMEDK2-bccc0db1.dat`
