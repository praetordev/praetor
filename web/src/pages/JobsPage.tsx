import React, { useState } from 'react';
import '../App.css';
import type { Job, JobTemplate, JobEvent } from '../types';
import Convert from 'ansi-to-html';


interface JobsPageProps {
    jobs: Job[];
    templates: JobTemplate[];
    logs: JobEvent[];
    onLaunchJob: (templateId: number) => void;
    onViewLogs: (runId: string) => void;
    onRefreshJobs: () => void;
    selectedRunID: string | null;
    onCloseLogs: () => void;
}

const JobsPage: React.FC<JobsPageProps> = ({
    jobs,
    templates,
    logs,
    onLaunchJob,
    onViewLogs,
    selectedRunID,
    onCloseLogs,
    onRefreshJobs
}) => {
    const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null);

    const handleLaunch = () => {
        if (selectedTemplateId) {
            onLaunchJob(selectedTemplateId);
        }
    };

    return (
        <div>
            <div className="header-actions">
                <h2 className="page-title">Jobs</h2>
                <button className="action-btn-sm" onClick={onRefreshJobs}>Refresh</button>
            </div>

            <div className="card launch-card" style={{ display: 'flex', gap: '1rem', alignItems: 'center' }}>
                <div style={{ flex: 1 }}>
                    <select
                        value={selectedTemplateId || ''}
                        onChange={(e) => setSelectedTemplateId(e.target.value ? parseInt(e.target.value) : null)}
                        style={{ width: '100%', padding: '10px' }}
                    >
                        <option value="">Select Template to Launch...</option>
                        {templates.map(t => (
                            <option key={t.id} value={t.id}>{t.name}</option>
                        ))}
                    </select>
                </div>
                <button onClick={handleLaunch} disabled={!selectedTemplateId}>Launch Job</button>
            </div>

            <div className="card">
                <table className="data-table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Name</th>
                            <th>Status</th>
                            <th>Time</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {jobs.map(job => (
                            <tr key={job.id}>
                                <td>#{job.id}</td>
                                <td>{job.name}</td>
                                <td><span className={`status-badge status-${job.status}`}>{job.status}</span></td>
                                <td>{new Date(job.created_at).toLocaleString()}</td>
                                <td>
                                    {job.current_run_id && (
                                        <button className="action-btn-sm" onClick={() => onViewLogs(job.current_run_id!)}>
                                            Logs
                                        </button>
                                    )}
                                </td>
                            </tr>
                        ))}
                        {jobs.length === 0 && (
                            <tr><td colSpan={5} style={{ textAlign: 'center' }}>No jobs found</td></tr>
                        )}
                    </tbody>
                </table>
            </div>

            {selectedRunID && (
                <div className="modal-overlay">
                    <div className="modal">
                        <div className="modal-header">
                            <h3>Execution Logs</h3>
                            <button onClick={onCloseLogs}>Close</button>
                        </div>
                        <div className="logs-container terminal" style={{ backgroundColor: '#000', padding: '1rem', overflowY: 'auto', flex: 1, fontFamily: 'monospace' }}>
                            <div className="terminal-output">
                                {logs
                                    .filter(log => log.stdout_snippet)
                                    .map((log, idx) => {
                                        // Create fresh converter for each line to prevent color bleeding
                                        const convert = new Convert({ newline: true });
                                        // Convert ANSI codes to HTML
                                        const html = convert.toHtml(log.stdout_snippet || '');

                                        // Heuristic coloring for lines that don't have ANSI codes or need highlighting
                                        const line = log.stdout_snippet || '';
                                        let color = '#ccc';

                                        // Priority: Failures first!
                                        if (line.startsWith('fatal:') || line.startsWith('failed:') || line.includes('FAILED!')) color = '#f87171';
                                        else if (line.startsWith('ok:') || line.includes('"changed": false')) color = '#4ade80';
                                        else if (line.startsWith('changed:') || line.includes('"changed": true')) color = '#facc15';
                                        else if (line.includes('PLAY [') || line.includes('TASK [') || line.includes('PLAY RECAP')) color = '#60a5fa';

                                        return (
                                            <div
                                                key={idx}
                                                style={{ margin: 0, color: color, whiteSpace: 'pre-wrap' }}
                                                dangerouslySetInnerHTML={{ __html: html }}
                                            />
                                        );
                                    })}
                            </div>
                            {logs.length === 0 && <p style={{ color: '#666' }}>Waiting for logs...</p>}
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};

export default JobsPage;
