// Names of the data-protection law and regulator for the operator's jurisdiction
// (LEGAL_JURISDICTION). Unknown jurisdictions get generic wording.

/** The data-protection regulator for the operator's jurisdiction, where we know it. */
export function regulator(jurisdiction: string): string | null {
  const j = jurisdiction.toLowerCase();
  if (j.includes("singapore")) return "the Personal Data Protection Commission (PDPC)";
  if (j.includes("united kingdom") || j === "uk" || j.includes("england")) return "the Information Commissioner's Office (ICO)";
  if (j.includes("australia")) return "the Office of the Australian Information Commissioner (OAIC)";
  if (j.includes("new zealand")) return "the Office of the Privacy Commissioner";
  if (j.includes("canada")) return "the Office of the Privacy Commissioner of Canada";
  return null;
}

/** The data-protection law of the operator's jurisdiction, where we know it. */
export function privacyLaw(jurisdiction: string): string | null {
  const j = jurisdiction.toLowerCase();
  if (j.includes("singapore")) return "the Personal Data Protection Act 2012";
  if (j.includes("united kingdom") || j === "uk" || j.includes("england")) return "the UK GDPR and the Data Protection Act 2018";
  if (j.includes("australia")) return "the Privacy Act 1988";
  if (j.includes("new zealand")) return "the Privacy Act 2020";
  if (j.includes("canada")) return "PIPEDA";
  return null;
}
