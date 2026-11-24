import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api, ApiError } from "../api";
import { JourneyProgress } from "../components/JourneyProgress";
import { JourneyView } from "../components/journey/JourneyView";
import type { Job } from "../types";

export function JobPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(
    () => api.getJob(id).then(setJob, (e) => setError(e instanceof ApiError && e.status === 404 ? "not-found" : e.message)),
    [id],
  );
  useEffect(() => void load(), [load]);


  if (error === "not-found") return <div className="narrow card" style={{ marginTop: 40 }}>That journey doesn&apos;t exist.</div>;
  if (error) return <div className="narrow card err" style={{ marginTop: 40 }}>{error}</div>;
  if (!job) return null;

  // Before the outline exists, show the full progress screen. Once it does, show the journey
  // itself with placeholders for the parts still being written.
  if (job.status === "pending" && !job.journey) return <JourneyProgress jobId={job.id} engine={job.engine} onFinished={load} onUpdate={load} />;
  const fill = () => api.fillJob(job.id).then(() => setJob({ ...job, status: "pending" }), (e) => setError(e.message));
  if (job.journey && (job.status === "pending" || job.status === "ready")) {
    const building = job.status === "pending";
    return (
      <JourneyView
        key={job.id}
        jobId={job.id}
        url={job.url}
        journey={job.journey}
        initialProgress={job.progress}
        initialPeople={job.people}
        shared={job.shared}
        building={building}
        onFill={fill}
        banner={building ? <JourneyProgress compact jobId={job.id} engine={job.engine} onFinished={load} onUpdate={load} /> : null}
      />
    );
  }
  if (job.status === "failed" || !job.journey) {
    return (
      <div className="narrow card" style={{ textAlign: "center", marginTop: 40 }}>
        <h2 style={{ marginTop: 0 }}>Couldn&apos;t build this journey</h2>
        <p className="err">{job.error || "Unknown error."}</p>
        <div className="row" style={{ justifyContent: "center", marginTop: 14 }}>
          <button className="btn" onClick={() => api.retryJob(job.id).then(() => setJob({ ...job, status: "pending" }), (e) => setError(e.message))}>Retry</button>
          <button className="btn ghost" onClick={() => api.deleteJob(job.id).then(() => navigate("/jobs"))}>Delete</button>
        </div>
      </div>
    );
  }
  return <JourneyView key={job.id} jobId={job.id} url={job.url} journey={job.journey} initialProgress={job.progress} initialPeople={job.people} shared={job.shared} />;
}
