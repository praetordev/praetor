import { useState, useEffect } from 'react'
import './App.css'

interface Job {
  id: number;
  name: string;
  status: string;
  created_at: string;
  current_run_id?: string;
}

interface JobEvent {
  seq: number;
  event_type: string;
  stdout_snippet: string;
  created_at: string;
  current_run_id?: string;
  task_name?: string;
}

interface Project {
  id: number;
  name: string;
  scm_url: string;
  scm_type: string;
}

interface JobTemplate {
  id: number;
  name: string;
  project_id?: number;
  playbook: string;
  organization_id: number;
}

function App() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [selectedRunID, setSelectedRunID] = useState<string | null>(null)
  const [logs, setLogs] = useState<JobEvent[]>([])

  // Projects State
  const [activeTab, setActiveTab] = useState('jobs')
  const [projects, setProjects] = useState<Project[]>([])
  const [newProjectName, setNewProjectName] = useState('')
  const [newProjectURL, setNewProjectURL] = useState('')

  // Templates State
  const [templates, setTemplates] = useState<JobTemplate[]>([])
  const [newTemplateName, setNewTemplateName] = useState('')
  const [newTemplateProjectId, setNewTemplateProjectId] = useState<number | null>(null)
  const [newTemplatePlaybook, setNewTemplatePlaybook] = useState('playbook.yml')
  const [selectedTemplateId, setSelectedTemplateId] = useState<number | null>(null)

  const fetchJobs = () => {
    fetch('/api/v1/jobs')
      .then(res => res.json())
      .then(data => setJobs(data || []))
      .catch(err => console.error("Failed to fetch jobs", err))
  }

  const fetchLogs = (runID: string) => {
    fetch(`/api/v1/jobs/runs/${runID}/events`)
      .then(res => res.json())
      .then(data => setLogs(data || []))
      .catch(err => console.error("Failed to fetch logs", err))
  }

  const fetchProjects = () => {
    fetch('/api/v1/projects')
      .then(res => res.json())
      .then(data => setProjects(data.items || []))
      .catch(err => console.error("Failed to fetch projects", err))
  }

  const fetchTemplates = () => {
    fetch('/api/v1/job-templates')
      .then(res => res.json())
      .then(data => setTemplates(data.items || []))
      .catch(err => console.error("Failed to fetch templates", err))
  }

  useEffect(() => {
    fetchJobs()
    fetchTemplates() // Load templates for job launch dropdown
    const interval = setInterval(fetchJobs, 2000)
    return () => clearInterval(interval)
  }, [])

  useEffect(() => {
    if (activeTab === 'projects') {
      fetchProjects()
    } else if (activeTab === 'templates') {
      fetchTemplates()
      fetchProjects() // Need projects for dropdown
    }
  }, [activeTab])

  useEffect(() => {
    if (selectedRunID) {
      fetchLogs(selectedRunID)
      const interval = setInterval(() => fetchLogs(selectedRunID), 2000)
      return () => clearInterval(interval)
    }
  }, [selectedRunID])

  const launchJob = () => {
    const payload: { name: string; unified_job_template_id?: number } = {
      name: `web-job-${Date.now()}`
    }
    if (selectedTemplateId) {
      payload.unified_job_template_id = selectedTemplateId
    }
    fetch('/api/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
      .then(() => fetchJobs())
      .catch(err => console.error("Failed to launch job", err))
  }

  const createTemplate = () => {
    if (!newTemplateName || !newTemplateProjectId) return
    fetch('/api/v1/job-templates', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: newTemplateName,
        project_id: newTemplateProjectId,
        playbook: newTemplatePlaybook,
        organization_id: 1
      })
    })
      .then(() => {
        setNewTemplateName('')
        setNewTemplateProjectId(null)
        setNewTemplatePlaybook('playbook.yml')
        fetchTemplates()
      })
      .catch(err => console.error("Failed to create template", err))
  }

  const createProject = () => {
    if (!newProjectName || !newProjectURL) return
    fetch('/api/v1/projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: newProjectName,
        scm_url: newProjectURL,
        scm_type: 'git',
        organization_id: 1
      })
    })
      .then(() => {
        setNewProjectName('')
        setNewProjectURL('')
        fetchProjects()
      })
      .catch(err => console.error("Failed to create project", err))
  }

  return (
    <div className="container">
      <h1>Praetor Automation</h1>

      <div className="tabs">
        <button className={activeTab === 'jobs' ? 'active' : ''} onClick={() => setActiveTab('jobs')}>Jobs</button>
        <button className={activeTab === 'templates' ? 'active' : ''} onClick={() => setActiveTab('templates')}>Templates</button>
        <button className={activeTab === 'projects' ? 'active' : ''} onClick={() => setActiveTab('projects')}>Projects</button>
      </div>

      {activeTab === 'jobs' && (
        <>
          <div className="card launch-card">
            <select
              value={selectedTemplateId || ''}
              onChange={(e) => setSelectedTemplateId(e.target.value ? parseInt(e.target.value) : null)}
            >
              <option value="">Use Default Playbook</option>
              {templates.map(t => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
            <button onClick={launchJob}>Launch Job</button>
          </div>

          <table>
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
                  <td>{job.id}</td>
                  <td>{job.name}</td>
                  <td className={`status-${job.status}`}>{job.status}</td>
                  <td>{new Date(job.created_at).toLocaleString()}</td>
                  <td>
                    {job.current_run_id && (
                      <button onClick={() => setSelectedRunID(job.current_run_id!)}>
                        View Logs
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {activeTab === 'templates' && (
        <>
          <div className="card template-form">
            <h3>Create Job Template</h3>
            <input
              placeholder="Template Name"
              value={newTemplateName}
              onChange={e => setNewTemplateName(e.target.value)}
            />
            <select
              value={newTemplateProjectId || ''}
              onChange={e => setNewTemplateProjectId(e.target.value ? parseInt(e.target.value) : null)}
            >
              <option value="">Select Project...</option>
              {projects.map(p => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <input
              placeholder="Playbook path (e.g., playbook.yml)"
              value={newTemplatePlaybook}
              onChange={e => setNewTemplatePlaybook(e.target.value)}
            />
            <button onClick={createTemplate}>Create Template</button>
          </div>

          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Name</th>
                <th>Playbook</th>
              </tr>
            </thead>
            <tbody>
              {templates.map(t => (
                <tr key={t.id}>
                  <td>{t.id}</td>
                  <td>{t.name}</td>
                  <td>{t.playbook || 'playbook.yml'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {activeTab === 'projects' && (
        <>
          <div className="card form-card">
            <h3>Add Project</h3>
            <input
              placeholder="Project Name"
              value={newProjectName}
              onChange={e => setNewProjectName(e.target.value)}
            />
            <input
              placeholder="Git URL"
              value={newProjectURL}
              onChange={e => setNewProjectURL(e.target.value)}
            />
            <button onClick={createProject}>Add</button>
          </div>

          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Name</th>
                <th>SCM URL</th>
                <th>Type</th>
              </tr>
            </thead>
            <tbody>
              {projects.map(proj => (
                <tr key={proj.id}>
                  <td>{proj.id}</td>
                  <td>{proj.name}</td>
                  <td>{proj.scm_url}</td>
                  <td>{proj.scm_type}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {selectedRunID && (
        <div className="modal-overlay">
          <div className="modal">
            <div className="modal-header">
              <h2>Execution Logs</h2>
              <button onClick={() => setSelectedRunID(null)}>Close</button>
            </div>
            <div className="logs-container terminal">
              <div className="terminal-output">
                {logs
                  .filter(log => log.stdout_snippet)
                  .map((log, idx) => {
                    const line = log.stdout_snippet;
                    let className = 'log-line-default';
                    if (line.startsWith('ok:') || line.includes('"changed": false')) {
                      className = 'log-line-ok';
                    } else if (line.startsWith('changed:') || line.includes('"changed": true')) {
                      className = 'log-line-changed';
                    } else if (line.startsWith('fatal:') || line.startsWith('failed:') || line.includes('FAILED')) {
                      className = 'log-line-failed';
                    } else if (line.includes('PLAY [') || line.includes('TASK [') || line.includes('PLAY RECAP')) {
                      className = 'log-line-header';
                    }
                    return <pre key={idx} className={className}>{line}</pre>;
                  })}
              </div>
              {logs.length === 0 && <p className="no-logs">Waiting for logs...</p>}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default App
