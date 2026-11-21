import { ASPECTS, type AspectKey } from "../../types";

export const aspectBy = Object.fromEntries(ASPECTS.map((a) => [a.key, a])) as Record<AspectKey, (typeof ASPECTS)[number]>;

export const safeHref = (u: string) => {
  try {
    return new URL(u).protocol === "https:" ? u : undefined;
  } catch {
    return undefined;
  }
};
