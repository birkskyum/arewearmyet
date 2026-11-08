# ASUS Vivobook S14 (S5408QA)

ACPI tables taken out of ASUS's Qualcomm SOC package 51014 (V1.304.9300.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/51014/SOCPackage_forWebSite_Qualcomm_Z_V1.304.9300.0_51014.exe), sha256 `5209523dae35e4fe4b80f866359cad0ed4731c2b8b9781bb8b4c19f1fb297826`), from the BIOS image S5408QA.304 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-37da9f23.dat`
- `SSDT-RamDisk-7ef53287.dat`
