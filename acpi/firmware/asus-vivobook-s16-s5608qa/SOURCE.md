# ASUS Vivobook S16 (S5608QA)

ACPI tables taken out of ASUS's Qualcomm SOC package 51015 (V1.304.9300.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/51015/SOCPackage_forWebSite_Qualcomm_Z_V1.304.9300.0_51015.exe), sha256 `beaffeeac45bbca13eb03fa325c5a31aa5fc858c9d37ee13615e4fdaaa1cf69c`), from the BIOS image S5608QA.304 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-5cd8ebf8.dat`
- `SSDT-RamDisk-7ef53287.dat`
