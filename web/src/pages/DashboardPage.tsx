import React from 'react';
import '../App.css';

interface Job {
    id: number;
    name: string;
    status: string;
    created_at: string;
}

interface DashboardProps {
    jobs: Job[];
}

const DashboardPage: React.FC<DashboardProps> = ({ jobs }) => {
    const totalJobs = jobs.length;
    const successfulJobs = jobs.filter(j => j.status === 'successful').length;
    const failedJobs = jobs.filter(j => j.status === 'failed').length;
    const successRate = totalJobs > 0 ? Math.round((successfulJobs / totalJobs) * 100) : 0;

    // Get recent 5 jobs
    const recentJobs = [...jobs].sort((a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    ).slice(0, 5);

    return (
        <div>
            <div className="header-actions">
                <h2 className="page-title">Dashboard</h2>
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '1.5rem', marginBottom: '2rem' }}>
                <div className="card">
                    <div style={{ fontSize: '0.9rem', color: '#a1a1aa' }}>Total Jobs</div>
                    <div style={{ fontSize: '2.5rem', fontWeight: 'bold' }}>{totalJobs}</div>
                </div>
                <div className="card">
                    <div style={{ fontSize: '0.9rem', color: '#a1a1aa' }}>Success Rate</div>
                    <div style={{ fontSize: '2.5rem', fontWeight: 'bold', color: '#4ade80' }}>{successRate}%</div>
                </div>
                <div className="card">
                    <div style={{ fontSize: '0.9rem', color: '#a1a1aa' }}>Failed Jobs</div>
                    <div style={{ fontSize: '2.5rem', fontWeight: 'bold', color: '#f87171' }}>{failedJobs}</div>
                </div>
            </div>

            <div className="card">
                <h3>Recent Activity</h3>
                <table className="data-table">
                    <thead>
                        <tr>
                            <th>Status</th>
                            <th>ID</th>
                            <th>Job Name</th>
                            <th>Time</th>
                        </tr>
                    </thead>
                    <tbody>
                        {recentJobs.map(job => (
                            <tr key={job.id}>
                                <td>
                                    <span className={`status-badge status-${job.status}`}>
                                        {job.status}
                                    </span>
                                </td>
                                <td>#{job.id}</td>
                                <td>{job.name}</td>
                                <td>{new Date(job.created_at).toLocaleString()}</td>
                            </tr>
                        ))}
                        {recentJobs.length === 0 && (
                            <tr><td colSpan={4} style={{ textAlign: 'center', padding: '2rem' }}>No jobs run yet</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
};

export default DashboardPage;
