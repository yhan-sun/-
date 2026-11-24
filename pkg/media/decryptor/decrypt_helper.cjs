const fs = require("fs");
const path = require("path");
const { Worker } = require("worker_threads");
const { execFileSync } = require("child_process");

function getInitRangeEnd(buf) {
  let pos = 0;
  while (pos + 8 <= buf.length) {
    const size = buf.readUInt32BE(pos);
    const type = buf.toString("ascii", pos + 4, pos + 8);
    if (type === "moov") {
      return pos + size - 1;
    }
    pos += size;
  }
  return -1;
}

function findFragments(buf) {
  const fragments = [];
  let pos = 0;
  while (pos + 8 <= buf.length) {
    const size = buf.readUInt32BE(pos);
    const type = buf.toString("ascii", pos + 4, pos + 8);
    if (size === 0 || size > buf.length - pos) break;

    if (type === "moof") {
      const moofStart = pos;
      const moofSize = size;
      const mdatPos = pos + size;
      if (mdatPos + 8 <= buf.length) {
        const mdatSize = buf.readUInt32BE(mdatPos);
        const mdatType = buf.toString("ascii", mdatPos + 4, mdatPos + 8);
        if (mdatType === "mdat") {
          const fragSize = moofSize + mdatSize;
          fragments.push(buf.subarray(moofStart, moofStart + fragSize));
          pos = moofStart + fragSize;
          continue;
        }
      }
    }
    pos += size;
  }
  return fragments;
}

function decryptTrack(fileBuf, isVideo, secretKey, workerWrapper) {
  return new Promise((resolve, reject) => {
    const initEnd = getInitRangeEnd(fileBuf);
    if (initEnd <= 0) {
      return reject(new Error("Failed to find moov box in track"));
    }
    const fragments = findFragments(fileBuf);
    if (fragments.length === 0) {
      return reject(new Error("No moof/mdat fragments found in track"));
    }

    const w = new Worker(workerWrapper, { eval: true });
    const decryptedBuffers = [];
    let currentFrag = 0;

    w.on("message", (msg) => {
      if (msg.method === "decryptHeader") {
        const res = msg.args?.data?.res;
        if (res) decryptedBuffers.push(Buffer.from(res));
        decryptNextFrag();
      } else if (msg.method === "decryptBody") {
        const res = msg.args?.data?.res;
        if (res) decryptedBuffers.push(Buffer.from(res));
        currentFrag++;
        if (currentFrag < fragments.length) {
          decryptNextFrag();
        } else {
          w.terminate();
          resolve(Buffer.concat(decryptedBuffers));
        }
      }
    });

    w.on("error", (err) => {
      w.terminate();
      reject(err);
    });

    function decryptNextFrag() {
      const frag = fragments[currentFrag];
      w.postMessage({
        method: "decryptBody",
        id: currentFrag + 10,
        args: {
          isVideo,
          buf: frag.buffer.slice(frag.byteOffset, frag.byteOffset + frag.byteLength),
          needRemux: true
        }
      });
    }

    w.postMessage({
      method: "updateValue",
      value: { secretKey, decryptKey: new Uint8Array() }
    });

    setTimeout(() => {
      const hBuf = fileBuf.subarray(0, initEnd + 1);
      w.postMessage({
        method: "decryptHeader",
        id: 1,
        buf: hBuf.buffer.slice(hBuf.byteOffset, hBuf.byteOffset + hBuf.byteLength),
        isVideo
      });
    }, 150);
  });
}

async function fetchBuffer(url) {
  const resp = await fetch(url, {
    headers: {
      "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
      "Referer": "https://hongguoduanju.com/"
    }
  });
  if (!resp.ok) throw new Error(`HTTP ${resp.status} fetching ${url}`);
  const ab = await resp.arrayBuffer();
  return Buffer.from(ab);
}

async function main() {
  let rawInput = "";
  if (process.argv[2] === "--config" && process.argv[3]) {
    const arg = process.argv[3];
    if (fs.existsSync(arg)) {
      rawInput = fs.readFileSync(arg, "utf8");
    } else {
      rawInput = arg;
    }
  } else {
    rawInput = fs.readFileSync(0, "utf8");
  }

  const config = JSON.parse(rawInput);
  const { videoUrl, audioUrl, secretKey, ffmpegPath, outputPath, workerJsPath } = config;

  if (!videoUrl || !audioUrl || !secretKey || !ffmpegPath || !outputPath) {
    throw new Error("Missing required config parameters: videoUrl, audioUrl, secretKey, ffmpegPath, outputPath");
  }

  const resolvedWorkerPath = workerJsPath || path.join(__dirname, "xg_worker.js");
  if (!fs.existsSync(resolvedWorkerPath)) {
    throw new Error(`Worker script not found at ${resolvedWorkerPath}`);
  }

  const workerCode = fs.readFileSync(resolvedWorkerPath, "utf8");
  const workerWrapper = `
const { parentPort } = require("worker_threads");
const self = global;
global.self = global;
global.onmessage = null;
global.postMessage = (data) => parentPort.postMessage(data);
global.importScripts = () => {};
global.location = { href: "https://hongguoduanju.com/" };

parentPort.on("message", (data) => {
  if (typeof global.onmessage === "function") {
    global.onmessage({ data });
  }
});

${workerCode}
`;

  // 1. Download video and audio tracks in parallel
  const [vBuf, aBuf] = await Promise.all([
    fetchBuffer(videoUrl),
    fetchBuffer(audioUrl)
  ]);

  // 2. Decrypt tracks in parallel
  const [decV, decA] = await Promise.all([
    decryptTrack(vBuf, true, secretKey, workerWrapper),
    decryptTrack(aBuf, false, secretKey, workerWrapper)
  ]);

  // Ensure output directory exists
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });

  const tmpV = outputPath + `.tmp_${Date.now()}_v.mp4`;
  const tmpA = outputPath + `.tmp_${Date.now()}_a.mp4`;
  fs.writeFileSync(tmpV, decV);
  fs.writeFileSync(tmpA, decA);

  try {
    execFileSync(ffmpegPath, [
      "-y",
      "-i", tmpV,
      "-i", tmpA,
      "-c", "copy",
      "-movflags", "+faststart",
      outputPath
    ], { stdio: "pipe" });

    const st = fs.statSync(outputPath);
    console.log(JSON.stringify({
      success: true,
      size: st.size,
      outputPath
    }));
  } finally {
    try { fs.unlinkSync(tmpV); } catch (e) {}
    try { fs.unlinkSync(tmpA); } catch (e) {}
  }
}

main().catch((err) => {
  console.error(JSON.stringify({
    success: false,
    error: err.message || String(err)
  }));
  process.exit(1);
});
