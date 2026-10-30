# Mica / Omarchy bring-up: V21 findings

Status as of 2026-10-30. Experimental Dell XPS Googlebook (Mica, Snapdragon
X1E80100), **not** the Windows Dell XPS 13 9345. This is a manually curated
report, not a raw device-log dump or a claim of supported hardware.

Device-tree contribution: [arewearmyet PR #3](https://github.com/birkskyum/arewearmyet/pull/3).
The earlier README's graphics-timeout milestone has been superseded by the
interactive display observations below. The contributed DTS itself is unchanged.

## Working baseline

- Booting an external USB through Depthcharge / Android boot image v4 at EL2.
  This is not a UEFI/UKI boot path.
- Linux `7.2.11-mica-usb-v2`, adapted from the
  [HP Quartz work at 8ed394a](https://github.com/ericman2001/hp-googlebook-linux/tree/8ed394a30c0fdc8a012b3a1131295ce18b48d0fe),
  with Mica-specific wiring and board-signed Mica ADSP firmware.
- Omarchy 4.5.0-rc2 ARM64 release-derived read-only lower filesystem with a
  disposable RAM overlay. ADSP starts before USB discovery; explicit
  dependency-ordered module loading is locked before the desktop starts.
- Internal NVMe remains disabled in the experimental DT. No internal install,
  partitioning, firmware update or dual-boot support is claimed.
- MSM/Adreno and Hyprland run. The tester physically saw a desktop, moved the
  cursor, and typed commands. Earlier V20 briefly appeared bright and then dim;
  that timing is approximate. V21 remained extremely dim / apparently unlit.
- Normal timed shutdown has been removed; a continuous safety monitor still
  requests shutdown on failed provider/module/internal-storage/root-RO checks.

## Priority 1: readable, stable backlight

Mica stock DT / runtime DT and our adapted tree agree on the principal wiring:
PMK8550 PWM channel 0, period 270000 ns, PMK8550 GPIO5 function `func3`,
PMC8380 at SPMI address 3 GPIO4 enable / GPIO10 power, and panel-enable TLMM70.
Do not substitute the Windows XPS 9345 or HP Quartz wiring based on model name.

Selected V21 observations (uptime since boot):

| Time | Operation | Requested brightness / max | `bl_power` | Result |
| --- | --- | --- | --- | --- |
| 100.35 s | Baseline read | 6 / 9 | 4 (off) | Read completed |
| 170.38 s | Standard backlight unblank | 6 / 9 | 0 (on) | Sysfs store returned 0 |
| 190.35 s | Standard maximum brightness | 9 / 9 | 0 (on) | Sysfs store returned 0 |
| Later snapshot | Read | 9 / 9 | 0 (on) | Still extremely dim by tester observation |

These values **do not measure emitted light**. In this kernel:

- `drivers/video/backlight/pwm_bl.c:pwm_backlight_update_status()` ignores the
  return from `pwm_apply_might_sleep()` and returns 0. Its power-on helper does
  not propagate a regulator-enable failure back through that sysfs store.
- There is no `get_brightness` callback; `actual_brightness` falls back to the
  requested value. A successful store and `actual_brightness=9` therefore do
  not establish that PWM or the power/enable path worked electrically.
- `drivers/pwm/core.c:pwm_dbg_show()` prints requested configuration **and**
  hardware readback through `pwm_get_state_hw()`.
- `drivers/leds/rgb/leds-qcom-lpg.c:lpg_pwm_get_state()` reads the PMIC's
  clock/duty/enable registers and propagates read errors. That is the next
  useful diagnostic, rather than guessing polarity or trying higher values.

Cached eDP driver state reports 2560×1600 at 120 Hz, four lanes at HBR3.
That state is not a light-output measurement either. The supported Omarchy
stay-awake setting did not solve the observed dimness.

**Questions for reviewers:** Is the PWM/power path above missing a required
PMK8550 pin configuration in mainline, or is panel/backlight lifecycle handling
the likely culprit? Stock GPIO5 configuration additionally names `output-low`
and `qcom,drive-strength=<3>`; these are comparison leads, not proposed blind
changes. Which board-specific evidence should we obtain before altering them?

## Priority 2: Wi-Fi, then authenticated SSH

- WCN power sequencing loads; PCIe4 reaches Gen3 ×2.
- The PCI device is enumerated and `ath12k_wifi7_pci` identifies WCN7850 hw2.0
  with 16 MSI vectors. Hardware detection is **not** the missing milestone.
- MHI reports power-on setup success and then "Wait for device to enter SBL
  or Mission mode" around 15.69 s. The collected stream contains no later
  firmware-version message. The later network snapshot has no Wi-Fi interface.
- `sshd` is active, but without a usable network this is not reachability proof.
  No association, DHCP, DNS, Internet or authenticated SSH success is claimed.
- Source/package audit found a concrete omission: `CONFIG_QRTR_MHI=m`, but the
  explicit V21 module closure lacks `kernel/net/qrtr/qrtr-mhi.ko`.
  This is the MHI `IPCR` QRTR/QMI transport. Dependencies `qrtr` and `mhi` are
  already present. It should load before ath12k and the irreversible module lock.
  Correcting this omission is justified, but is **not proof** that it explains
  the entire MHI boot problem.
- Historical V3 using broader autoload initialized firmware and registered a
  wireless interface. It did not establish successful network association.

**Question for reviewers:** After including QRTR-MHI, if the interface is still
absent, which MHI execution-environment/state transitions should distinguish
firmware boot failure from IPCR/QMI transport failure? The bounded `states`
debugfs callback reads cached state/counters; raw register dumps and reset/debug
writes are not planned.

## Next physical test

Preserve the working kernel/DT/graphics/release-root baseline. Add bounded,
one-shot PWM requested/actual readback around the existing backlight stages,
only after verifying that the sole registered PWM provider is the expected
PMK8550 driver. Include QRTR-MHI in the exact locked module closure and collect
bounded MHI `states`. Keep automatic private Wi-Fi provisioning and key-only
SSH. No speculative PWM polarity, GPIO, panel-timing or firmware changes.

If these diagnostics show successful PWM output configuration but the panel is
still dark, investigate pin routing and the power/enable path before changing
unrelated desktop configuration. Register readback still is not a photometric
measurement or proof that the signal reaches the panel.

## Sharing and limitations

This report intentionally excludes credentials, SSH private keys, device serials,
MAC/IP addresses, trial identifiers, screenshots, raw logs, vendor firmware,
stock DTS dumps, and provisioned images. Those images contain private network
provisioning and must not be published. Charging, audio, Bluetooth, suspend,
internal installation, dual boot and long-term reliability remain unqualified.
