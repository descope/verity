export const SEVERITY_ORDER = ["CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"] as const;

export type Severity = (typeof SEVERITY_ORDER)[number];

export const SEVERITY_COLORS: Record<string, { bg: string; text: string; bar: string }> = {
  CRITICAL: { bg: "bg-red-50", text: "text-red-800", bar: "bg-red-500" },
  HIGH: { bg: "bg-orange-50", text: "text-orange-800", bar: "bg-orange-500" },
  MEDIUM: { bg: "bg-yellow-50", text: "text-yellow-800", bar: "bg-yellow-500" },
  LOW: { bg: "bg-green-50", text: "text-green-800", bar: "bg-green-500" },
  UNKNOWN: { bg: "bg-gray-50", text: "text-gray-800", bar: "bg-gray-400" },
};

export function severityIndex(sev: string): number {
  const idx = SEVERITY_ORDER.indexOf(sev as Severity);
  return idx === -1 ? SEVERITY_ORDER.length : idx;
}

export function sortBySeverity<T>(items: T[], getSeverity: (item: T) => string): T[] {
  return [...items].sort((a, b) => severityIndex(getSeverity(a)) - severityIndex(getSeverity(b)));
}
