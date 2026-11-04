# HP OmniBook Ultra 14-kg0xxx

ACPI tables, hardware IDs and other data from an owner's hardware report, made with the [Omarchy Arm Installer](https://github.com/turbineBMW/omarchy-arm-installer) on BIOS F.06 (board 8F03, SKU D32V6UA#ABA). The installer took out the serial numbers, the system UUID and the MAC addresses, and the DSDT's SOSN, the chip's serial number, is zeroed with the table's checksum intact. Disassemble with `iasl -d dsdt.dat`.

`hwids.json` holds the 14 hardware IDs systemd-stub can match, recomputed from the report's SMBIOS table, under the compatible of the posted tree. omacom/omarchy-pkgs#2142 carries them in Omarchy's kernel package.

From the same report:

- `smbios/dmi.bin` is the SMBIOS table. Read it with `dmidecode --from-dump smbios/dmi.bin`.
- `edid/0-SDC4215-internal.bin` is the panel's EDID.
- `devices.json` lists the 284 devices Windows found, with their hardware IDs, ACPI paths, drivers and properties. Binary values are base64.
- `firmware.json` lists the firmware files of the 183 driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
