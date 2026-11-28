# Hardware report hp/hp-omnibook-5-laptop-14-he0xxx/97863638

From https://github.com/turbineBMW/omarchy-hardware-reports/tree/8ad795bace951c22fe99ca5f28b82467eb815de2/reports/hp/hp-omnibook-5-laptop-14-he0xxx/97863638, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected.
It still had the processor's or the panel's serial number, which is blanked here.
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
