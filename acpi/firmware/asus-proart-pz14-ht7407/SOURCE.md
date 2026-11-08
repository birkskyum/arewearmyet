# ASUS ProArt PZ14

ACPI tables taken out of ASUS's Qualcomm SOC package 52286 (V1.309.4500.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/52286/SOCPackage_forWebSite_Qualcomm_Z_V1.309.4500.0_52286.exe), sha256 `379ff10e604bd5c31c0c649ba473a7b7553ff8c3c351ba1b3cb7ee4b94da13fc`), from the BIOS image HT7407NA.309 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-SDM8480-f75e4e0e.dat`
- `SSDT-RamDisk-98a1af0a.dat`
