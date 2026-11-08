# ASUS Vivobook S14 Flip

ACPI tables taken out of ASUS's Qualcomm SOC package 51031 (V1.305.9300.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/51031/SOCPackage_forWebSite_Qualcomm_Z_V1.305.9300.0_51031.exe), sha256 `a2b5dbd232923eed04949904da3281b43f1cea1a0b17c9887e041bcafe300035`), from the BIOS image TP5408QA.305 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-9a0cdded.dat`
- `SSDT-RamDisk-7ef53287.dat`
