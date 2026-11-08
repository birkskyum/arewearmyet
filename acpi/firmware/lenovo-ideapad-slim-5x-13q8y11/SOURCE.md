# Lenovo IdeaPad Slim 5x 13 Gen 11

ACPI tables taken out of Lenovo's BIOS update [tscn23ww.7z](https://download.lenovo.com/consumer/mobiles/tscn23ww.7z), from the TSCN23WW image inside it. The same image serves the 13 and 15 inch models. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-64805a67.dat`
- `BERT-QCOMEDK2-73183051.dat`
- `BGRT-QCOMEDK2-53eb02ef.dat`
- `CSRT-QCOMEDK2-9246684b.dat`
- `DBG2-QCOMEDK2-636f7d8b.dat`
- `DSDT-SDM8480-e4981be4.dat`
- `FACP-QCOMEDK2-7bd02f50.dat`
- `GTDT-QCOMEDK2-c9dbe8dd.dat`
- `IORT-QCOMEDK2-80c48181.dat`
- `MCFG-QCOMEDK2-7a529ca6.dat`
- `PPTT-QCOMEDK2-6b7f28cd.dat`
- `SPCR-QCOMEDK2-de2280cc.dat`
- `TPM2-QCOMEDK2-12b2990c.dat`
