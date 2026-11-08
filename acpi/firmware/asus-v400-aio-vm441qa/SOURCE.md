# ASUS V400 AiO (VM441QA)

ACPI tables taken out of ASUS's Qualcomm SOC package 52327 (V1.301.8600.0, [download](https://dlcdnets.asus.com/pub/ASUS/All-In-One/Image/Driver/DriverPackage/52327/SOCPackage_forWebSite_CC_Qualcomm_F_V1.301.8600.0_52327.exe), sha256 `eccefbecd75a2bf8d938cb8589f761192b143972baeeb46367bddd8018179fa3`), from the BIOS image VM441QA_301.cap inside it. Package 50530 (BIOS 3.00) has the same DSDT. They come from the maker's own download, not from a running machine, so they hold no per-unit values (the SoC serial and ID fields are placeholders the firmware fills in at boot) and no MSDM. Fields the firmware patches at boot can differ from a dump of a running machine. An owner's dump is still wanted for its hardware IDs, panel and driver list. Disassemble with `iasl -d`.

- `DSDT-SDM8380-377e0a1d.dat`
- `SSDT-RamDisk-7ef53287.dat`
