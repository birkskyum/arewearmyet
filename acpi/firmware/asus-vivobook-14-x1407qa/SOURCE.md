# ASUS Vivobook 14

ACPI tables taken out of ASUS's Qualcomm SOC package 50054 (V1.314.8800.0, [download](https://dlcdnets.asus.com/pub/ASUS/nb/Image/Driver/DriverPackage/50054/SOCPackage_forWebSite_Qualcomm_Z_V1.314.8800.0_50054.exe), sha256 `caf0cbd096a4eca8788f4a910a4d34ca1c2174b9e0875d0feaada7fa45356f17`), from the BIOS image X1407QA.314 inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-Notebook-075d2098.dat`
- `SSDT-RamDisk-7ef53287.dat`
