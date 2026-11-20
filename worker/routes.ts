/**
 * Every route under /api/, in the order the reference describes them (src/lib/api.md, shown at
 * /docs/api). GET /api answers with this list, and test/docs.test.ts fails when the reference and
 * this list name different routes, or when a route here isn't served.
 */
export const DOCS = 'https://arewearmyet.com/docs/api';

export const ROUTES = [
  ['GET', '/api', 'This list'],
  ['GET', '/api/checks', 'Every check, with what works means, what is measured, what the owner does and where to look when it fails'],
  ['GET', '/api/models', 'Every computer model with its id, page and API link'],
  ['GET', '/api/models/<id>', 'One model, with booting, findings, owners\' results, fixes, notes and wanted checks'],
  ['GET', '/api/match', 'Which model a computer is, from its hardware IDs, device tree or firmware strings'],
  ['GET', '/api/owners', 'Every model\'s owners\' results at once, and the computers that matched no model'],
  ['GET', '/api/runs/<id>', 'One run as stored'],
  ['GET', '/api/fixes', 'What one pull request changed, over every computer that measured it'],
  ['GET', '/api/notes', 'Notes, which are problems, workarounds and pointers to work elsewhere'],
  ['GET', '/api/notes/<id>', 'One note with its answers and its logs'],
  ['GET', '/api/notes/<id>/logs/<name>', 'One log of a note, as plain text'],
  ['POST', '/api/runs', 'Send a run of results from one computer'],
  ['POST', '/api/runs/<id>/results', 'Add results to a run, from the same computer'],
  ['POST', '/api/fixes', 'Link a fix to a model, with what it changed as measured before and after'],
  ['POST', '/api/needs', 'Say that a part works on a computer because of a change made on it'],
  ['POST', '/api/notes', 'Write a note, with logs'],
  ['POST', '/api/notes/<id>/replies', 'Answer a note'],
  ['POST', '/api/notes/<id>/status', 'Mark your own problem solved, or open again'],
  ['GET', '/api/review', 'The newest runs, fixes and notes, hidden ones included. Needs the admin token'],
  ['POST', '/api/review/hide', 'Hide or show a run, fix, note or machine. Needs the admin token'],
  ['POST', '/api/firmware/missing', 'Which of a list of firmware files the drop doesn\'t hold yet'],
  ['PUT', '/api/firmware/files/<sha256>', 'Store one firmware file under its SHA-256'],
  ['POST', '/api/firmware/uploads/<sha256>', 'Start sending a file too large for one request'],
  ['PUT', '/api/firmware/uploads/<sha256>/<upload>/<part>', 'Send one part of it'],
  ['POST', '/api/firmware/uploads/<sha256>/<upload>/complete', 'Join the parts and store the file'],
  ['POST', '/api/firmware/reports', 'Store the list of a computer\'s firmware files and get its code'],
] as const;
