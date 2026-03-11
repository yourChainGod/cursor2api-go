import { createWorker } from 'tesseract.js';

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf8');
}

function toImageSource(block) {
  if (!block || block.type !== 'image' || !block.source?.data) return null;
  if (block.source.type === 'base64') {
    const mime = block.source.media_type || 'image/jpeg';
    return `data:${mime};base64,${block.source.data}`;
  }
  if (block.source.type === 'url') {
    return block.source.data;
  }
  return null;
}

async function main() {
  const raw = await readStdin();
  const payload = JSON.parse(raw || '{}');
  const images = Array.isArray(payload.images) ? payload.images : [];
  const worker = await createWorker('eng+chi_sim');
  let combinedText = '';

  try {
    for (let i = 0; i < images.length; i++) {
      const imageSource = toImageSource(images[i]);
      if (!imageSource) continue;
      try {
        const { data: { text } } = await worker.recognize(imageSource);
        combinedText += `--- Image ${i + 1} OCR Text ---\n${(text || '').trim() || '(No text detected in this image)'}\n\n`;
      } catch (err) {
        combinedText += `--- Image ${i + 1} ---\n(Failed to parse image with local OCR: ${err?.message || String(err)})\n\n`;
      }
    }
  } finally {
    await worker.terminate();
  }

  process.stdout.write(JSON.stringify({ text: combinedText.trim() }));
}

main().catch((err) => {
  process.stdout.write(JSON.stringify({ error: err?.message || String(err) }));
  process.exit(1);
});
