import { useEffect, useState } from "react";

// Phones get list views instead of node graphs: graphs need room to read and precise taps.
const QUERY = "(max-width: 720px)";

export function useMobile() {
  const [mobile, setMobile] = useState(() => typeof window !== "undefined" && window.matchMedia(QUERY).matches);
  useEffect(() => {
    const mq = window.matchMedia(QUERY);
    const on = () => setMobile(mq.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return mobile;
}
