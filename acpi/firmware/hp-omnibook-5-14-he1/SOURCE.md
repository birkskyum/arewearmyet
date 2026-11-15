# HP OmniBook 5 14 (2026, 14-he1)

ACPI tables taken out of HP's SoftPaq [sp173974](https://ftp.hp.com/pub/softpaq/sp173501-174000/sp173974.exe) (Qualcomm Driver Pack 4500.X6, sha256 a65b78efb4ac44579622376f2bb6996e2fcdc6e878a81ec4386a672eddb82321), from the UEFI capsule src/Driver/BIOS_Update/BIOS_eFuse_Payload.cap (board 8F47, Insyde BIOS F.06). The DSDT and the Qualcomm tables are the "ACPI" container in the capsule's LZMA-compressed ACPI ELF; the four SSDTs are Insyde's, from the capsule's compressed DXE volume. Board 8F47 is also the OmniBook 5 16 (2026, 16-bf1)'s, and the same pack and capsule serve both. The DSDT selects between the Glymur and Mahua dies on `\_SB.SDFE` (0xA1, 0xA8), and the Kalambo chip (X2P-42-100) is the Mahua die. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial SOSN is the firmware's placeholder 0xAAAAAAAABBBBBBBB, and the ID fields and the checksums of the Qualcomm tables are filled in at boot) and no MSDM (the capsule's MSDM is an empty Insyde template and is left out). Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-dea80d45.dat`
- `BERT-QCOMEDK2-c2f9d2d8.dat`
- `BGRT-QCOMEDK2-9fd0386c.dat`
- `CSRT-QCOMEDK2-720cc408.dat`
- `DBG2-QCOMEDK2-0f91e64d.dat`
- `DSDT-SDM8480-9237ce8c.dat`
- `FACP-QCOMEDK2-6a407175.dat`
- `GTDT-QCOMEDK2-24dbe14d.dat`
- `IORT-QCOMEDK2-d4f3eed6.dat`
- `MCFG-QCOMEDK2-f12de3ad.dat`
- `PPTT-QCOMEDK2-15551956.dat`
- `SPCR-QCOMEDK2-cff96fa1.dat`
- `SSDT-HWIDTabl-47253211.dat`
- `SSDT-RamDisk-d5be37a3.dat`
- `SSDT-WMITable-30354550.dat`
- `SSDT-WMITest-16f0fd63.dat`
- `TPM2-QCOMEDK2-bccc0db1.dat`
