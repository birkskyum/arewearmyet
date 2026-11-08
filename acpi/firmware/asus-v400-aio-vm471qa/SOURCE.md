# ASUS V400 AiO (VM471QA)

ACPI tables taken out of ASUS's Qualcomm SOC package 50150 (V1.202.8600.0, [download](https://dlcdnets.asus.com/pub/ASUS/All-In-One/Image/Driver/DriverPackage/50150/SOCPackage_forWebSite_CC_Qualcomm_F_V1.202.8600.0_50150.exe), sha256 `a0f68997b7a812d00b39c9398a89eed4342be7e70fa59eb6632bcf15cd871e25`), from the BIOS image VM471QA_202.cap inside it. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-SDM8380-377e0a1d.dat`
- `SSDT-RamDisk-7ef53287.dat`
