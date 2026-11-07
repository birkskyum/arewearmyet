# Hardware report hp/hp-omnibook-3-laptop-16-bz0xxx/6cff1e36

From https://github.com/turbineBMW/omarchy-hardware-reports/tree/821b76eda2e7fcb808e515f18aa97ff609a83b0d/reports/hp/hp-omnibook-3-laptop-16-bz0xxx/6cff1e36, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected.
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
