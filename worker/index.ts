/**
 * The site's pages, plus a private drop for firmware the Omarchy Arm Installer
 * finds in a laptop's Windows drivers, when its owner opts in.
 *
 * The bucket has no public URL and this Worker only writes to it: nothing here
 * lists or returns a stored file. Files are kept by their SHA-256, so a file
 * many laptops share is stored once, and R2 rejects a body whose hash differs
 * from the one in its URL. The installer asks which files are missing, sends
 * those, then sends the list of every file with its driver package, and gets a
 * code back for the owner's hardware report.
 *
 * A request body can't pass 100 MB on this plan, so a bigger file comes in
 * parts: an R2 multipart upload under incoming/, which is then copied to its
 * place under files/ with the same SHA-256 check, so R2 does the hashing
 * rather than the Worker.
 */

const MAX_FILE = 95 * 1024 * 1024;
const MAX_LARGE = 1024 * 1024 * 1024;
const MAX_PARTS = 100;
const MAX_HASHES = 40;
const MAX_REPORT_FILES = 5000;
const MAX_REPORT = 2 * 1024 * 1024;
const SHA256 = /^[0-9a-f]{64}$/;
// Paths are only kept as data (files are stored by hash), so any Windows
// file name goes, short of control characters and backslashes
const UNSAFE = /[\u0000-\u001f\u007f\\/]/;
const MACHINE_KEYS = ['manufacturer', 'product', 'family', 'sku', 'baseboard', 'bios'] as const;
const CODE_ALPHABET = 'ABCDEFGHJKMNPQRSTVWXYZ23456789';

export default {
  async fetch(request, env): Promise<Response> {
    const url = new URL(request.url);
    if (!url.pathname.startsWith('/api/firmware/')) return env.ASSETS.fetch(request);
    if (env.FIRMWARE_UPLOADS !== 'on') return json({ error: 'Uploads are closed.' }, 503);

    const ip = request.headers.get('cf-connecting-ip') ?? 'local';
    const { success } = await env.UPLOAD_LIMIT.limit({ key: ip });
    if (!success) return json({ error: 'Too many requests, try again in a minute.' }, 429);

    const route = url.pathname.slice('/api/firmware/'.length);
    if (route === 'missing' && request.method === 'POST') return missing(request, env);
    if (route.startsWith('files/') && request.method === 'PUT') return putFile(route.slice('files/'.length), request, env);
    if (route.startsWith('uploads/')) return upload(route.slice('uploads/'.length).split('/'), request, env);
    if (route === 'reports' && request.method === 'POST') return putReport(request, env);
    return json({ error: 'Not found.' }, 404);
  },
} satisfies ExportedHandler<Env>;

/** Which of up to 40 hashes the bucket doesn't hold yet. */
async function missing(request: Request, env: Env): Promise<Response> {
  const body = await readJson(request, 64 * 1024);
  const hashes = body && typeof body === 'object' && 'sha256' in body ? body.sha256 : null;
  if (!Array.isArray(hashes) || hashes.length > MAX_HASHES || !hashes.every((h) => typeof h === 'string' && SHA256.test(h)))
    return json({ error: `Send {"sha256": [...]} with at most ${MAX_HASHES} lowercase SHA-256 hashes.` }, 400);
  const unique = [...new Set(hashes as string[])];
  const held = await Promise.all(unique.map((h) => env.FIRMWARE.head(`files/${h}`)));
  return json({ missing: unique.filter((_, i) => !held[i]) });
}

/** Stores one file under its SHA-256, which R2 checks against the body. */
async function putFile(hash: string, request: Request, env: Env): Promise<Response> {
  if (!SHA256.test(hash)) return json({ error: 'The path must end in the lowercase SHA-256 of the file.' }, 400);
  const length = Number(request.headers.get('content-length'));
  if (!Number.isInteger(length) || length <= 0) return json({ error: 'Content-Length is required.' }, 411);
  if (length > MAX_FILE) return json({ error: `Files over ${MAX_FILE} bytes aren't taken.` }, 413);
  if (!request.body) return json({ error: 'The file is missing.' }, 400);

  const key = `files/${hash}`;
  if (await env.FIRMWARE.head(key)) return json({ stored: false });
  try {
    await env.FIRMWARE.put(key, request.body, { sha256: hash });
  } catch {
    return json({ error: "The file doesn't match its SHA-256." }, 400);
  }
  return json({ stored: true }, 201);
}

/**
 * A file over MAX_FILE, in parts:
 * POST uploads/<sha256> with {"size"} starts it and answers {"uploadId"};
 * PUT uploads/<sha256>/<uploadId>/<part> sends part 1, 2, ..., each the same
 * size but the last, at least 5 MiB and at most MAX_FILE, and answers {"etag"};
 * POST uploads/<sha256>/<uploadId>/complete with {"parts": [{"partNumber", "etag"}]}
 * joins them, checks the SHA-256 and stores the file.
 */
async function upload(path: string[], request: Request, env: Env): Promise<Response> {
  const [hash, uploadId, part] = path;
  if (!SHA256.test(hash ?? '')) return json({ error: 'The path must name the lowercase SHA-256 of the file.' }, 400);
  const key = `files/${hash}`;
  const incoming = `incoming/${hash}`;

  if (path.length === 1 && request.method === 'POST') {
    const body = await readJson(request, 1024);
    const size = body && typeof body === 'object' && 'size' in body ? body.size : null;
    if (typeof size !== 'number' || !Number.isInteger(size) || size <= MAX_FILE || size > MAX_LARGE)
      return json({ error: `Send {"size"} for a file over ${MAX_FILE} and up to ${MAX_LARGE} bytes.` }, 400);
    if (await env.FIRMWARE.head(key)) return json({ stored: false });
    const started = await env.FIRMWARE.createMultipartUpload(incoming);
    return json({ uploadId: started.uploadId }, 201);
  }
  if (!uploadId) return json({ error: 'Not found.' }, 404);
  const multipart = env.FIRMWARE.resumeMultipartUpload(incoming, uploadId);

  if (path.length === 3 && part !== 'complete' && request.method === 'PUT') {
    const number = Number(part);
    if (!Number.isInteger(number) || number < 1 || number > MAX_PARTS) return json({ error: `Parts are numbered 1 to ${MAX_PARTS}.` }, 400);
    const length = Number(request.headers.get('content-length'));
    if (!Number.isInteger(length) || length <= 0) return json({ error: 'Content-Length is required.' }, 411);
    if (length > MAX_FILE) return json({ error: `Parts over ${MAX_FILE} bytes aren't taken.` }, 413);
    if (!request.body) return json({ error: 'The part is missing.' }, 400);
    try {
      const uploaded = await multipart.uploadPart(number, request.body);
      return json({ etag: uploaded.etag });
    } catch {
      return json({ error: 'No such upload.' }, 404);
    }
  }

  if (path.length === 3 && part === 'complete' && request.method === 'POST') {
    const body = await readJson(request, 64 * 1024);
    const parts = body && typeof body === 'object' && 'parts' in body ? body.parts : null;
    if (!Array.isArray(parts) || parts.length === 0 || parts.length > MAX_PARTS
        || !parts.every((p) => p && Number.isInteger(p.partNumber) && typeof p.etag === 'string'))
      return json({ error: 'Send {"parts": [{"partNumber", "etag"}]}.' }, 400);
    let joined: R2Object;
    try {
      joined = await multipart.complete(parts as R2UploadedPart[]);
    } catch {
      return json({ error: "The parts couldn't be joined." }, 400);
    }
    try {
      if (await env.FIRMWARE.head(key)) return json({ stored: false });
      if (joined.size > MAX_LARGE) return json({ error: `Files over ${MAX_LARGE} bytes aren't taken.` }, 413);
      const object = await env.FIRMWARE.get(incoming);
      if (!object) return json({ error: 'The joined file is missing.' }, 500);
      // R2 hashes what it is given and refuses a mismatch, as for one PUT
      const { readable, writable } = new FixedLengthStream(object.size);
      const copied = object.body.pipeTo(writable);
      try {
        await Promise.all([env.FIRMWARE.put(key, readable, { sha256: hash }), copied]);
      } catch {
        return json({ error: "The file doesn't match its SHA-256." }, 400);
      }
      return json({ stored: true }, 201);
    } finally {
      await env.FIRMWARE.delete(incoming);
    }
  }
  return json({ error: 'Not found.' }, 404);
}

/** Stores the list of files a laptop sent and returns its code. */
async function putReport(request: Request, env: Env): Promise<Response> {
  const body = await readJson(request, MAX_REPORT);
  if (!body || typeof body !== 'object') return json({ error: 'Send the report as JSON.' }, 400);
  const { installer, machine, files } = body as Record<string, unknown>;

  if (typeof installer !== 'string' || installer.length > 40) return json({ error: 'installer must name its version.' }, 400);
  const cleanMachine: Record<string, string> = {};
  if (machine && typeof machine === 'object') {
    for (const k of MACHINE_KEYS) {
      const v = (machine as Record<string, unknown>)[k];
      if (typeof v === 'string' && v.length <= 120) cleanMachine[k] = v;
    }
  }
  if (!cleanMachine.manufacturer || !cleanMachine.product) return json({ error: 'machine needs manufacturer and product.' }, 400);
  if (!Array.isArray(files) || files.length === 0 || files.length > MAX_REPORT_FILES)
    return json({ error: `files must list 1 to ${MAX_REPORT_FILES} files.` }, 400);

  const cleanFiles = [];
  for (const f of files) {
    const { folder, name, size, sha256 } = (f ?? {}) as Record<string, unknown>;
    if (typeof folder !== 'string' || !segment(folder) || typeof name !== 'string' || name.split('/').length > 8 || !name.split('/').every(segment))
      return json({ error: `Bad file path: ${String(folder)}/${String(name)}` }, 400);
    if (typeof size !== 'number' || !Number.isInteger(size) || size < 0) return json({ error: `Bad size for ${folder}/${name}` }, 400);
    if (typeof sha256 !== 'string' || !SHA256.test(sha256)) return json({ error: `Bad SHA-256 for ${folder}/${name}` }, 400);
    cleanFiles.push({ folder, name, size, sha256, uploaded: size <= MAX_LARGE });
  }

  // The same machine sending the same files again gets its first code back,
  // so a second run makes no second report; any other driver changes it
  const digestKey = `digests/${await listDigest(cleanMachine, cleanFiles)}`;
  const known = await env.FIRMWARE.get(digestKey);
  if (known) return json({ code: await known.text(), known: true });

  const code = newCode();
  const report = { code, received: new Date().toISOString(), installer, machine: cleanMachine, files: cleanFiles };
  await env.FIRMWARE.put(`reports/${code}.json`, JSON.stringify(report, null, 1), {
    httpMetadata: { contentType: 'application/json' },
  });
  await env.FIRMWARE.put(digestKey, code);
  return json({ code, known: false }, 201);
}

async function listDigest(machine: Record<string, string>, files: { folder: string; name: string; sha256: string }[]): Promise<string> {
  const lines = files.map((f) => `${f.folder}/${f.name} ${f.sha256}`).sort();
  const text = JSON.stringify([MACHINE_KEYS.map((k) => machine[k] ?? ''), lines]);
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text));
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('');
}

function segment(part: string): boolean {
  return part.length > 0 && part.length <= 160 && part !== '.' && part !== '..' && !UNSAFE.test(part);
}

async function readJson(request: Request, limit: number): Promise<unknown> {
  const length = Number(request.headers.get('content-length') ?? 0);
  if (length > limit) return null;
  const text = await request.text();
  if (text.length > limit) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** Ten characters with no look-alikes, like FW-K7Q2M9XH4P. */
function newCode(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(10));
  return 'FW-' + Array.from(bytes, (b) => CODE_ALPHABET[b % CODE_ALPHABET.length]).join('');
}

function json(value: unknown, status = 200): Response {
  return Response.json(value, { status, headers: { 'cache-control': 'no-store' } });
}
