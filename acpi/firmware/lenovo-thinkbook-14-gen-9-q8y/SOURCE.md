# Lenovo ThinkBook 14 Gen 9

ACPI tables taken out of Lenovo's BIOS update [uecn20ww.exe](https://download.lenovo.com/consumer/mobiles/uecn20ww.exe), from the UECN20WW image inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-dea80d45.dat`
- `BERT-QCOMEDK2-c2f9d2d8.dat`
- `BGRT-QCOMEDK2-9fd0386c.dat`
- `CSRT-QCOMEDK2-720cc408.dat`
- `DBG2-QCOMEDK2-0f91e64d.dat`
- `DSDT-SDM8480-5d20d92d.dat`
- `FACP-QCOMEDK2-6a407175.dat`
- `GTDT-QCOMEDK2-24dbe14d.dat`
- `IORT-QCOMEDK2-d4f3eed6.dat`
- `MCFG-QCOMEDK2-f12de3ad.dat`
- `PPTT-QCOMEDK2-15551956.dat`
- `SDEV-ESS-de163f0e.dat`
- `SDEV-QCOMEDK2-d4b255a7.dat`
- `SPCR-QCOMEDK2-cff96fa1.dat`
- `SSDT-LfcECCom-121c9817.dat`
- `SSDT-RamDisk-7ef53287.dat`
- `SSDT-WmiTable-980dd528.dat`
- `TPM2-QCOMEDK2-bccc0db1.dat`
