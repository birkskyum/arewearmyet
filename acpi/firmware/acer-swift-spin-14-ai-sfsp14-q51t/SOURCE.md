# Acer Swift Spin 14 AI (SFSP14-Q51T)

ACPI tables taken out of Acer's BIOS 1.06 [BIOS_Acer_1.06_A_A.zip](https://global-download.acer.com/GDFiles/BIOS/BIOS/BIOS_Acer_1.06_A_A.zip?acerid=639239248959001923&Step1=&Step2=&Step3=SWIFT%20SFSP14-Q51T&OS=ALL&LC=en&BC=ACER&SC=PA_6) (sha256 `4c72e13a1a67bfd5b036457998370296672f4c0d7fd60c8e95f7502eca98aef4`), from the Insyde capsule V1.06.cap inside its 7-Zip installer V1.06.exe, from the LZMA and XZ sections of its firmware volumes. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot, and the static tables carry a zero checksum until then) and no MSDM. The DSDT's SOSN placeholder is zeroed and its checksum put right. Insyde's MSDM and WPBT templates in the image are left out. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-dea80d45.dat`
- `BERT-QCOMEDK2-c2f9d2d8.dat`
- `BGRT-QCOMEDK2-9fd0386c.dat`
- `CSRT-QCOMEDK2-720cc408.dat`
- `DBG2-QCOMEDK2-0f91e64d.dat`
- `DSDT-SDM8480-bf4edf4a.dat`
- `FACP-QCOMEDK2-6a407175.dat`
- `GTDT-QCOMEDK2-24dbe14d.dat`
- `IORT-2KDEMOCQ-d9286b3e.dat`
- `IORT-QCOMEDK2-d4f3eed6.dat`
- `MCFG-QCOMEDK2-f12de3ad.dat`
- `PPTT-QCOMEDK2-15551956.dat`
- `SDEV-QCOMEDK2-a7a90db9.dat`
- `SDEV-QCOMEDK2-d4b255a7.dat`
- `SPCR-QCOMEDK2-cff96fa1.dat`
- `SSDT-ACRPRDCT-a09a69d7.dat`
- `SSDT-RamDisk-7ef53287.dat`
- `TPM2-QCOMEDK2-bccc0db1.dat`
