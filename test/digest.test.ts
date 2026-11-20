// The nightly digest's comparison of owners' reports with the written findings (scripts/digest.ts).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { added, digest, render, type Findings, type Known, type Owners } from '../scripts/digest.ts';

const known: Known = {
  checks: [{ id: 'wifi', title: 'Wi-Fi' }, { id: 'suspend', title: 'Sleep and wake' }, { id: 'camera', title: 'Webcam' }, { id: 'usb4', title: 'USB4 and Thunderbolt' }, { id: 'speakers', title: 'Speakers' }],
  models: [{ id: 'yoga', vendor: 'Lenovo', name: 'Yoga Slim 7x', generation: 'x1' }, { id: 'omnibook', vendor: 'HP', name: 'OmniBook X 14', generation: 'x1' }],
};
const findings: Findings = {
  families: { x1: { checks: { usb4: { state: 'broken' } } } },
  models: { yoga: { checks: { wifi: { state: 'works' }, camera: { state: 'broken' } } } },
};
const check = (states: Record<string, number>, extra: object = {}) => ({ machines: Object.values(states).reduce((a, b) => a + b, 0), states, how: {}, latest: [{ state: Object.keys(states)[0], note: null, kernel: '7.3.0-72-omarchy' }], ...extra });

test('sorts what owners report into a work list', () => {
  const owners: Owners = {
    models: {
      yoga: {
        machines: 3,
        checks: {
          // Written up as working, one owner has it broken
          wifi: check({ works: 2, broken: 1 }),
          // Written up as broken, owners report it fixed
          camera: check({ works: 1 }, { fixed: { machines: 1, from: '7.3.0-59-omarchy', to: '7.3.0-72-omarchy', since: '2026-11-17T10:00:00.000Z' } }),
          // Nothing written up for the model, two machines agree
          speakers: check({ works: 2 }),
          // The family's finding says broken, an owner has it working
          usb4: check({ works: 1 }),
        },
      },
      omnibook: {
        machines: 1,
        checks: { suspend: check({ broken: 1 }, { regressed: { machines: 1, from: '7.3.0-67-omarchy', to: '7.3.0-72-omarchy', since: '2026-11-17T10:00:00.000Z' } }) },
      },
    },
    unmatched: [{ hardware: { maker: 'Acme', product: 'Unheard Of 14' }, compatible: 'acme,unheard', machines: 1, runs: 2, latest: '2026-11-18T00:00:00.000Z' }],
  };
  const result = digest(owners, findings, known);
  assert.equal(result.machines, 4);
  assert.equal(result.models, 2);
  assert.equal(result.regressions.length, 1);
  assert.match(result.regressions[0], /HP OmniBook X 14.*Sleep and wake.*7\.3\.0-67-omarchy to 7\.3\.0-72-omarchy/);
  assert.equal(result.contradictions.length, 2);
  assert.match(result.contradictions[0], /Wi-Fi\*\* is written up as working, owners report 2 works, 1 broken/);
  assert.match(result.contradictions[1], /USB4 and Thunderbolt\*\* is written up as broken in its chip family's finding/);
  assert.equal(result.fixed.length, 1);
  assert.match(result.fixed[0], /Webcam\*\* works since 7\.3\.0-72-omarchy/);
  assert.equal(result.ready.length, 1);
  assert.match(result.ready[0], /Speakers\*\* works on 2 machines\. `speakers: \{ state: works, checked: \d{4}-\d{2} \}`/);
  // The family-level USB4 report and the OmniBook's sleep have no finding of the model's own
  assert.equal(result.first.length, 2);
  assert.match(result.unmatched[0], /Acme Unheard Of 14, device tree `acme,unheard`, with 2 runs from 1 machine/);

  const body = render(result);
  assert.match(body, /^Owners' reports from 4 machines across 2 models/);
  for (const heading of ['Regressions', 'Findings owners contradict', 'Fixed, but still written up as broken', 'Ready to write up', 'First reports', "Machines the board doesn't know"]) assert.ok(body.includes(`## ${heading}`), heading);
});

test('says nothing when reports only confirm the findings', () => {
  const owners: Owners = { models: { yoga: { machines: 1, checks: { wifi: check({ works: 1 }) } } }, unmatched: [] };
  assert.equal(render(digest(owners, findings, known)), '');
});

test('tells which items are new since the last digest', () => {
  assert.deepEqual(added('intro\n- a\n- b\n- c\n', 'older intro\n- a\n- c\n'), ['- b']);
});
