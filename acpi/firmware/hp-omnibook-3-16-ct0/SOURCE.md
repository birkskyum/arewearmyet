# HP OmniBook 3 16 (16-ct0)

The DSDT taken out of HP's SoftPaq [sp174797](https://ftp.hp.com/pub/softpaq/sp174501-175000/sp174797.exe) (Qualcomm Driver Pack r02010.1, sha256 c0b43804f921d3e74494753fe8891b77bc09fe347f949d25365e50aabc543569), from the UEFI capsule src/Driver/1.BIOS_Update/SystemFirmware.bin (boards 8FD0 and 8FD1, the Snapdragon C C1-10-100). The capsule holds it uncompressed. The other Qualcomm tables in it have their ID fields and checksums filled in at boot, so they are left out, and HP's Insyde SSDTs are in a volume this extraction doesn't unpack. It comes from the maker's own download, not from a running machine, so it holds no per-unit values. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-SDM7280-18125702.dat`
