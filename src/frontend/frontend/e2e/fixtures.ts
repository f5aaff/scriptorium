/** silentWave is a one-second, mono 8 kHz PCM original for codec smoke coverage. */
export function silentWave(): Buffer {
  const dataSize = 8000 * 2;
  const bytes = Buffer.alloc(44 + dataSize);
  bytes.write('RIFF', 0); bytes.writeUInt32LE(36 + dataSize, 4); bytes.write('WAVEfmt ', 8);
  bytes.writeUInt32LE(16, 16); bytes.writeUInt16LE(1, 20); bytes.writeUInt16LE(1, 22);
  bytes.writeUInt32LE(8000, 24); bytes.writeUInt32LE(16000, 28); bytes.writeUInt16LE(2, 32); bytes.writeUInt16LE(16, 34);
  bytes.write('data', 36); bytes.writeUInt32LE(dataSize, 40);
  return bytes;
}

/** twoPagePDF is a valid PDF with inert-by-default actions and a standard font.
 * The browser smoke test verifies rendering/navigation without activating either
 * the document's JavaScript OpenAction or its external link annotation.
 */
export function twoPagePDF(): Buffer {
  const stream = (text: string) => {
    const body = `BT /F1 16 Tf 72 720 Td (${text}) Tj ET\n`;
    return `<< /Length ${body.length} >>\nstream\n${body}endstream`;
  };
  const objects = [
    '<< /Type /Catalog /Pages 2 0 R /OpenAction 8 0 R >>',
    '<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 4 0 R /Annots [9 0 R] >>',
    stream('FirstViewerPhrase programming and libraries'),
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>',
    stream('SecondViewerPhrase searchable page two'),
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    "<< /S /JavaScript /JS (app.alert\\('PDF scripting must stay disabled'\\)) >>",
    '<< /Type /Annot /Subtype /Link /Rect [72 690 250 710] /A << /S /URI /URI (https://example.invalid/pdf-external) >> >>'
  ];
  let pdf = '%PDF-1.4\n';
  const offsets = [0];
  objects.forEach((object, index) => {
    offsets.push(pdf.length);
    pdf += `${index + 1} 0 obj\n${object}\nendobj\n`;
  });
  const xref = pdf.length;
  pdf += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`;
  pdf += offsets.slice(1).map(offset => `${String(offset).padStart(10, '0')} 00000 n \n`).join('');
  pdf += `trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(pdf);
}
