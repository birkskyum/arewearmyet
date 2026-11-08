# ASUS Vivobook S16 (S3607NA)

ACPI tables taken out of ASUS's Qualcomm SOC package 48976 (V1.309.4500.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/48976/SOCPackage_forWebSite_Qualcomm_Z_V1.309.4500.0_48976.exe), sha256 `3be6e7b17ea332999beee48cca8bc70a40bfd1d76ba740056b404bed3adbada0`), from the BIOS image S3607NA.309 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-d6ab7e45.dat`
- `SSDT-RamDisk-7ef53287.dat`
