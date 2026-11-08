# ASUS Vivobook S16 Flip

ACPI tables taken out of ASUS's Qualcomm SOC package 51032 (V1.305.9300.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/51032/SOCPackage_forWebSite_Qualcomm_Z_V1.305.9300.0_51032.exe), sha256 `d46215ac1ca85f29d7b9abfe7ee1ac37d1568c1e47219b6092695774208b0edc`), from the BIOS image TP5608QA.305 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-7caa33d2.dat`
- `SSDT-RamDisk-7ef53287.dat`
