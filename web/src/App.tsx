import { useState, useEffect } from 'react'
import './App.css'
import type { Job, JobEvent, Project, JobTemplate, Inventory, Host, Group } from './types'

// Components
import Layout from './components/Layout'
import Sidebar from './components/Sidebar'

// Pages
import DashboardPage from './pages/DashboardPage'
import JobsPage from './pages/JobsPage'
import ProjectsPage from './pages/ProjectsPage'
import TemplatesPage from './pages/TemplatesPage'
import InventoriesPage from './pages/InventoriesPage'

function App() {
  // Global Navigation State
  const [activeTab, setActiveTab] = useState('dashboard')

  // Data States
  const [jobs, setJobs] = useState<Job[]>([])
  const [selectedRunID, setSelectedRunID] = useState<string | null>(null)
  const [logs, setLogs] = useState<JobEvent[]>([])

  const [projects, setProjects] = useState<Project[]>([])
  const [templates, setTemplates] = useState<JobTemplate[]>([])
  const [inventories, setInventories] = useState<Inventory[]>([])

  // Inventory/Host/Group Selection States
  const [selectedInventoryId, setSelectedInventoryId] = useState<number | null>(null)
  const [hosts, setHosts] = useState<Host[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [selectedGroupId, setSelectedGroupId] = useState<number | null>(null)
  const [groupHosts, setGroupHosts] = useState<Host[]>([])

  // --- API FETCHERS ---

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

  const fetchInventories = () => {
    fetch('/api/v1/inventories')
      .then(res => res.json())
      .then(data => setInventories(data.items || []))
      .catch(err => console.error("Failed to fetch inventories", err))
  }

  const fetchHosts = (inventoryId: number) => {
    fetch(`/api/v1/inventories/${inventoryId}/hosts`)
      .then(res => res.json())
      .then(data => setHosts(data || []))
      .catch(err => console.error("Failed to fetch hosts", err))
  }

  const fetchGroups = (inventoryId: number) => {
    fetch(`/api/v1/inventories/${inventoryId}/groups`)
      .then(res => res.json())
      .then(data => setGroups(data || []))
      .catch(err => console.error("Failed to fetch groups", err))
  }

  const fetchGroupHosts = (groupId: number) => {
    fetch(`/api/v1/groups/${groupId}/hosts`)
      .then(res => res.json())
      .then(data => setGroupHosts(data || []))
      .catch(err => console.error("Failed to fetch group hosts", err))
  }

  // --- EFFECTS ---

  // Initial Load & Polling
  useEffect(() => {
    fetchJobs()
    fetchTemplates()
    fetchProjects()
    fetchInventories()

    const interval = setInterval(fetchJobs, 2000)
    return () => clearInterval(interval)
  }, [])

  // Poll Logs if viewing
  useEffect(() => {
    if (selectedRunID) {
      fetchLogs(selectedRunID)
      const interval = setInterval(() => fetchLogs(selectedRunID), 2000)
      return () => clearInterval(interval)
    }
  }, [selectedRunID])

  // --- ACTION HANDLERS ---

  const handleLaunchJob = (templateId: number) => {
    const payload = {
      name: `web-auto-job-${Date.now()}`,
      unified_job_template_id: templateId
    }
    fetch('/api/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })
      .then(() => fetchJobs())
      .catch(err => console.error("Failed to launch job", err))
  }

  const handleCreateProject = (name: string, url: string) => {
    fetch('/api/v1/projects', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name,
        scm_url: url,
        scm_type: 'git',
        organization_id: 1
      })
    })
      .then(() => fetchProjects())
      .catch(err => console.error("Failed to create project", err))
  }

  const handleSyncProject = (id: number) => {
    return fetch(`/api/v1/projects/${id}/sync`, { method: 'POST' })
      .then(res => res.json())
      .then(data => {
        if (data.success) {
          fetchProjects() // Refresh
          const cleanRev = data.revision ? data.revision.trim() : 'N/A'
          const cleanMsg = data.commit_msg ? data.commit_msg.trim() : 'N/A'
          alert(`✅ Project Synced Successfully!\n\nVerified Commit: ${cleanRev}\nSubject: ${cleanMsg}`)
          return true
        } else {
          alert('Sync Failed: ' + data.error)
          return false
        }
      })
      .catch(err => {
        alert('Sync Request Failed: ' + err)
        return false
      })
  }

  const handleCreateTemplate = (name: string, projectId: number, inventoryId: number, playbook: string) => {
    fetch('/api/v1/job-templates', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, project_id: projectId, inventory_id: inventoryId, playbook })
    })
      .then(res => res.json())
      .then(data => {
        if (data.id) {
          alert('Template Created Successfully!')
          fetchTemplates()
        } else {
          alert('Failed to create template')
        }
      })
  }

  const handleUpdateTemplate = (id: number, name: string, projectId: number, inventoryId: number, playbook: string) => {
    fetch(`/api/v1/job-templates/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, project_id: projectId, inventory_id: inventoryId, playbook })
    })
      .then(res => res.json())
      .then(data => {
        if (data.id) {
          alert('Template Updated Successfully!')
          fetchTemplates()
        } else {
          alert('Failed to update template')
        }
      })
      .catch(err => alert('Update Request Failed: ' + err))
  }
  const handleCreateInventory = (name: string) => {
    fetch('/api/v1/inventories', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, organization_id: 1 })
    })
      .then(res => res.json())
      .then(data => {
        fetchInventories()
        setSelectedInventoryId(data.id)
      })
      .catch(err => console.error("Failed to create inventory", err))
  }

  const handleCreateHost = (name: string, vars: string) => {
    if (!selectedInventoryId) return
    let variables = {}
    try {
      if (vars) variables = JSON.parse(vars)
    } catch {
      alert('Invalid JSON for variables')
      return
    }
    fetch(`/api/v1/inventories/${selectedInventoryId}/hosts`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, variables })
    })
      .then(() => fetchHosts(selectedInventoryId))
      .catch(err => console.error("Failed to create host", err))
  }

  const handleDeleteHost = (hostId: number) => {
    if (!selectedInventoryId) return
    if (!confirm('Are you sure you want to delete this host?')) return

    fetch(`/api/v1/hosts/${hostId}`, {
      method: 'DELETE',
    })
      .then(res => {
        if (res.ok) {
          fetchHosts(selectedInventoryId)
        } else {
          console.error("Failed to delete host")
          alert("Failed to delete host")
        }
      })
      .catch(err => console.error("Failed to delete host", err))
  }

  const handleCreateGroup = (name: string) => {
    if (!selectedInventoryId) return
    fetch(`/api/v1/inventories/${selectedInventoryId}/groups`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name })
    })
      .then(() => fetchGroups(selectedInventoryId))
      .catch(err => console.error("Failed to create group", err))
  }

  const handleAddHostToGroup = (hostId: number) => {
    if (!selectedGroupId) return
    fetch(`/api/v1/groups/${selectedGroupId}/hosts`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host_id: hostId })
    })
      .then(() => fetchGroupHosts(selectedGroupId))
      .catch(err => console.error("Failed to add host to group", err))
  }

  const handleSelectInventory = (id: number | null) => {
    setSelectedInventoryId(id)
    if (id) {
      fetchHosts(id)
      fetchGroups(id)
    } else {
      setHosts([])
      setGroups([])
    }
    setSelectedGroupId(null)
  }

  const handleSelectGroup = (id: number | null) => {
    setSelectedGroupId(id)
    if (id) fetchGroupHosts(id)
    else setGroupHosts([])
  }

  // --- RENDER ---

  const renderContent = () => {
    switch (activeTab) {
      case 'dashboard':
        return <DashboardPage jobs={jobs} />
      case 'jobs':
        return (
          <JobsPage
            jobs={jobs}
            templates={templates}
            logs={logs}
            onLaunchJob={handleLaunchJob}
            onViewLogs={setSelectedRunID}
            onCloseLogs={() => setSelectedRunID(null)}
            onRefreshJobs={fetchJobs}
            selectedRunID={selectedRunID}
          />
        )
      case 'projects':
        return (
          <ProjectsPage
            projects={projects}
            onCreateProject={handleCreateProject}
            onSyncProject={handleSyncProject}
          />
        )
      case 'templates':
        return (
          <TemplatesPage
            templates={templates}
            projects={projects}
            inventories={inventories}
            onCreateTemplate={handleCreateTemplate}
            onUpdateTemplate={handleUpdateTemplate}
          />
        )
      case 'inventories':
        return (
          <InventoriesPage
            inventories={inventories}
            hosts={hosts}
            groups={groups}
            groupHosts={groupHosts}
            selectedInventoryId={selectedInventoryId}
            selectedGroupId={selectedGroupId}
            onSelectInventory={handleSelectInventory}
            onSelectGroup={handleSelectGroup}
            onCreateInventory={handleCreateInventory}
            onCreateHost={handleCreateHost}
            onCreateGroup={handleCreateGroup}
            onAddHostToGroup={handleAddHostToGroup}
            onDeleteHost={handleDeleteHost}
          />
        )
      default:
        return <div style={{ color: 'white' }}>Page Not Found</div>
    }
  }

  return (
    <Layout
      sidebar={
        <Sidebar activeTab={activeTab} onTabChange={setActiveTab} />
      }
    >
      {renderContent()}
    </Layout>
  )
}

export default App
