# Lenovo ThinkCentre neo 50q QC

ACPI tables taken out of Lenovo's BIOS update [m60jy4busa.exe](https://download.lenovo.com/pccbbs/thinkcentre_bios/m60jy4busa.exe), from the M60KT4BA image inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-9e370a66.dat`
- `BGRT-QCOMEDK2-5f3020bb.dat`
- `CSRT-QCOMEDK2-c0cd18aa.dat`
- `DBG2-QCOMEDK2-422fa67f.dat`
- `DSDT-SDM8380-97d1a28e.dat`
- `FACP-QCOMEDK2-7a3805a0.dat`
- `GTDT-QCOMEDK2-d00f2534.dat`
- `IORT-2KDEMOCQ-73da0b37.dat`
- `IORT-QCOMEDK2-8a2c7dac.dat`
- `MCFG-QCOMEDK2-47ebeecd.dat`
- `PPTT-QCOMEDK2-91d10e7c.dat`
- `SDEV-QCOMEDK2-807c8893.dat`
- `SPCR-QCOMEDK2-4e247584.dat`
- `SSDT-LfcECCom-724b0df2.dat`
- `SSDT-QcomWmiT-2411c459.dat`
- `SSDT-RamDisk-7ef53287.dat`
- `SSDT-WmiTable-6c9da910.dat`
- `TPM2-QCOMEDK2-09cb8009.dat`
