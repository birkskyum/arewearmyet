# HP OmniBook X 14 (14-kr0) and OmniBook X 16 (16-cp0)

ACPI tables taken out of HP's SoftPaq [sp172628](https://ftp.hp.com/pub/softpaq/sp172501-173000/sp172628.exe) (Qualcomm Driver Pack 8900, sha256 3df5f938c7597ca2fba7ecefae5aa8a1d0cf13a0c22284f91812d535b6c273f4), from the UEFI capsule src/OSPart/BIOS_Update/BIOS_eFuse_Payload.cap (board 8F45, Insyde BIOS F.03). The DSDT and the Qualcomm tables are the "ACPI" container in the capsule's LZMA-compressed ACPI ELF (\ACPI\ACPI.lzma); the four SSDTs are Insyde's, from the capsule's compressed DXE volume. The 14-kr0 and the 16-cp0 share board 8F45 and this capsule, so these tables serve both. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial SOSN is the firmware's placeholder 0xAAAAAAAABBBBBBBB, and the ID fields and the checksums of the Qualcomm tables are filled in at boot) and no MSDM (the capsule's MSDM is an empty Insyde template and is left out). Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `APIC-QCOMEDK2-9e370a66.dat`
- `BGRT-QCOMEDK2-5f3020bb.dat`
- `CSRT-QCOMEDK2-c0cd18aa.dat`
- `DBG2-QCOMEDK2-422fa67f.dat`
- `DSDT-SDM8380-a25ee740.dat`
- `FACP-QCOMEDK2-7eebc830.dat`
- `GTDT-QCOMEDK2-d00f2534.dat`
- `IORT-QCOMEDK2-8a2c7dac.dat`
- `MCFG-QCOMEDK2-47ebeecd.dat`
- `PPTT-QCOMEDK2-91d10e7c.dat`
- `SPCR-QCOMEDK2-4e247584.dat`
- `SSDT-HWIDTabl-c2418a47.dat`
- `SSDT-RamDisk-7ef53287.dat`
- `SSDT-WMITable-77b06f17.dat`
- `SSDT-WMITest-7de2aae8.dat`
- `TPM2-QCOMEDK2-09cb8009.dat`
