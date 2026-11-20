// GSM 03.38 basic character set and its extension table. A message that
// stays within them is encoded in 7 bits (160 characters per SMS); any other
// character switches the whole message to UCS-2 (70 per SMS).
const GSM7 =
  "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà";
const GSM7_EXTENDED = "^{}\\[~]|€\f";

export interface SmsEncoding {
  encoding: "GSM-7" | "UCS-2";
  segments: number;
  /** Characters left in the current segment. */
  remaining: number;
}

export function smsSegments(text: string): SmsEncoding {
  let units = 0;
  let gsm = true;
  for (const ch of text) {
    if (GSM7.includes(ch)) units += 1;
    else if (GSM7_EXTENDED.includes(ch)) units += 2;
    else {
      gsm = false;
      break;
    }
  }

  if (!gsm) {
    units = text.length; // UTF-16 code units
    return measure("UCS-2", units, 70, 67);
  }
  return measure("GSM-7", units, 160, 153);
}

function measure(encoding: SmsEncoding["encoding"], units: number, single: number, multi: number): SmsEncoding {
  if (units === 0) return { encoding, segments: 0, remaining: single };
  if (units <= single) return { encoding, segments: 1, remaining: single - units };
  const segments = Math.ceil(units / multi);
  return { encoding, segments, remaining: segments * multi - units };
}
