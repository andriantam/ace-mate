import type { Dashboard, DocumentRow, Job, People, Session, Verification } from "./types";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: "same-origin", headers: {} };
  if (body instanceof FormData) init.body = body;
  else if (body !== undefined) {
    (init.headers as Record<string, string>)["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch {
    throw new ApiError(0, "Couldn't reach the server. Check your connection.");
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, (data as { error?: string }).error ?? "Something went wrong.");
  return data as T;
}

export const api = {
  session: () => request<Session>("GET", "/api/session"),
  logout: () => request<{ ok: true }>("POST", "/api/auth/logout"),
  dashboard: () => request<Dashboard>("GET", "/api/dashboard"),
  documents: () => request<{ documents: DocumentRow[] }>("GET", "/api/documents"),
  documentProgress: (id: string, after: number) =>
    request<{ events: UploadEvent[]; next: number; done: boolean }>("GET", `/api/documents/${id}/progress?after=${after}`),
  jobProgress: (id: string, after: number) =>
    request<{ events: JobEvent[]; next: number; done: boolean }>("GET", `/api/jobs/${id}/progress?after=${after}`),
  deleteDocument: (id: string) => request<{ ok: true }>("DELETE", `/api/documents/${id}`),
  acceptTerms: (version: string, marketing: boolean) => request<{ ok: true }>("POST", "/api/consent", { accept: true, version, marketing }),
  setMarketing: (optIn: boolean) => request<{ ok: true }>("PUT", "/api/consent/marketing", { optIn }),
  createJob: (b: { url: string; jdText: string; notes: string }) => request<{ id: string }>("POST", "/api/jobs", b),
  getJob: (id: string) => request<Job>("GET", `/api/jobs/${id}`),
  deleteJob: (id: string) => request<{ ok: true }>("DELETE", `/api/jobs/${id}`),
  fillJob: (id: string) => request<{ ok: true }>("POST", `/api/jobs/${id}/fill`),
  retryJob: (id: string) => request<{ ok: true }>("POST", `/api/jobs/${id}/retry`),
  setProgress: (id: string, item: string, done: boolean) => request<{ ok: true }>("PUT", `/api/jobs/${id}/progress`, { item, done }),
  savePeople: (id: string, people: People) => request<{ ok: true }>("PUT", `/api/jobs/${id}/people`, people),
  deleteAccount: (confirm: string) => request<{ ok: true }>("DELETE", "/api/account", { confirm }),
};


/** Sent with step "retrying": the attempt failed in a way that may pass, and will run again. */
export type RetryInfo = { attempt?: number; of?: number; seconds?: number };

export type UploadEvent =
  | { type: "upload"; loaded: number; total: number }
  | {
      type: "progress";
      step: "received" | "fetch" | "fetched" | "extract" | "render" | "read" | "read_done" | "reconcile" | "saving" | "retrying";
      kind?: DocumentRow["kind"];
      detail?: string;
      pages?: number;
      chars?: number;
      method?: "text" | "visual" | "pdf";
      methods?: ("text" | "visual" | "pdf")[];
      ok?: boolean;
    } & RetryInfo
  | { type: "done"; verification: Verification | null }
  | { type: "error"; error: string };

export type DocumentInput = { file?: File; text?: string; url?: string; ackModel: string };

/**
 * Sends a document and resolves with its id as soon as the server has it; processing then
 * continues in the background. XMLHttpRequest is used because fetch can't report upload progress.
 */
export function addDocument(input: DocumentInput, onUpload: (loaded: number, total: number) => void): Promise<string> {
  const form = new FormData();
  form.append("ackModel", input.ackModel);
  if (input.file) form.append("resume", input.file);
  else if (input.url) form.append("url", input.url);
  else form.append("text", input.text ?? "");
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/documents");
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onUpload(e.loaded, e.total);
    };
    xhr.onload = () => {
      let data: { id?: string; error?: string } = {};
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        /* not JSON */
      }
      if (xhr.status === 202 && data.id) resolve(data.id);
      else reject(new ApiError(xhr.status, data.error ?? "Something went wrong."));
    };
    xhr.onerror = () => reject(new ApiError(0, "Couldn't reach the server. Check your connection."));
    xhr.send(form);
  });
}

export type JobEvent =
  | {
      type: "progress";
      step: "queued" | "fetch" | "fetched" | "outline" | "outline_done" | "batch_done" | "retrying";
      detail?: string;
      chars?: number;
      nodes?: number;
      done?: number;
      total?: number;
      aspect?: string;
    } & RetryInfo
  | { type: "done" }
  | { type: "error"; error: string };
