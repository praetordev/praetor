import React from 'react';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import TemplatesPage from './TemplatesPage';

const mocks = vi.hoisted(() => ({
  bulkLaunchJobs: vi.fn(),
  getLaunchConfiguration: vi.fn(),
  previewLaunch: vi.fn(),
  launchJob: vi.fn(),
}));

vi.mock('../services/api', () => ({
  newIdempotencyKey: () => 'ui-bulk-launch-test',
  unwrap: (value: any) => Array.isArray(value) ? value : value?.items ?? [],
  api: {
    getTemplates: vi.fn().mockResolvedValue([
      { id: 11, organization_id: 5, name: 'Deploy web', playbook: 'web.yml', unified_job_template_id: 101 },
      { id: 12, organization_id: 5, name: 'Deploy db', playbook: 'db.yml', unified_job_template_id: 102 },
    ]),
    getWorkflows: vi.fn().mockResolvedValue([]),
    getJobs: vi.fn().mockResolvedValue([]),
    getWorkflowJobs: vi.fn().mockResolvedValue([]),
    getProjects: vi.fn().mockResolvedValue([]),
    getInventories: vi.fn().mockResolvedValue([]),
    getCredentials: vi.fn().mockResolvedValue([]),
    getExecutionPacks: vi.fn().mockResolvedValue([]),
    getOrganizations: vi.fn().mockResolvedValue([{ id: 5, name: 'Engineering' }]),
    bulkLaunchJobs: mocks.bulkLaunchJobs,
    getJobLaunchConfiguration: mocks.getLaunchConfiguration,
    previewJobLaunch: mocks.previewLaunch,
    launchJob: mocks.launchJob,
  },
}));

afterEach(() => {
  cleanup();
  mocks.bulkLaunchJobs.mockReset();
  mocks.getLaunchConfiguration.mockReset();
  mocks.previewLaunch.mockReset();
  mocks.launchJob.mockReset();
});

describe('fleet-scale browser journey', () => {
  it('selects templates, reports mixed results, and retries only the failed item', async () => {
    mocks.bulkLaunchJobs
      .mockResolvedValueOnce({
        idempotency_key: 'launch-first',
        complete: true,
        results: [
          { index: 0, identifier: 'Deploy web', status: 'accepted', http_status: 201, job_id: 91 },
          { index: 1, identifier: 'Deploy db', status: 'rejected', http_status: 403, error: 'Launch not permitted' },
        ],
      })
      .mockResolvedValueOnce({
        idempotency_key: 'launch-retry',
        complete: true,
        results: [
          { index: 0, identifier: 'Deploy db', status: 'accepted', http_status: 201, job_id: 92 },
        ],
      });

    render(
      <MemoryRouter initialEntries={['/templates/org/5']}>
        <Routes>
          <Route path="/templates/org/:orgId" element={<TemplatesPage />} />
        </Routes>
      </MemoryRouter>,
    );

    fireEvent.click(await screen.findByRole('checkbox', { name: 'Select all visible job templates' }));
    fireEvent.click(screen.getByRole('button', { name: 'Launch selected' }));

    expect(await screen.findByText('1 succeeded · 1 failed')).toBeTruthy();
    expect(mocks.bulkLaunchJobs).toHaveBeenNthCalledWith(1, [
      expect.objectContaining({ identifier: 'Deploy web', unified_job_template_id: 101 }),
      expect.objectContaining({ identifier: 'Deploy db', unified_job_template_id: 102 }),
    ], 'ui-bulk-launch-test');

    fireEvent.click(screen.getByRole('button', { name: 'Retry failed' }));
    await waitFor(() => expect(mocks.bulkLaunchJobs).toHaveBeenCalledTimes(2));
    expect(mocks.bulkLaunchJobs).toHaveBeenNthCalledWith(2, [
      expect.objectContaining({ identifier: 'Deploy db', unified_job_template_id: 102 }),
    ], 'ui-bulk-launch-test');
    expect(await screen.findByText('1 succeeded · 0 failed')).toBeTruthy();
  });

  it('launches a catalog template through server preview and navigates to its job', async () => {
    mocks.getLaunchConfiguration.mockResolvedValue({
      template: { id: 12, unified_job_template_id: 102, name: 'Deploy db', organization_id: 5 },
      prompts: { inventory: false, credential: false, variables: false, limit: false, survey: false },
      defaults: { extra_vars: {}, limit: '' },
      inventories: [],
      credentials: [],
    });
    mocks.previewLaunch.mockResolvedValue({
      template: { id: 12, unified_job_template_id: 102, name: 'Deploy db' },
      extra_vars: {},
      limit: '',
      inventory_host_count: 0,
      inventory_host_sample: [],
      limit_applied_at_execution: true,
    });
    mocks.launchJob.mockResolvedValue({ id: 91, status: 'pending' });

    render(
      <MemoryRouter initialEntries={['/templates/org/5']}>
        <Routes>
          <Route path="/templates/org/:orgId" element={<TemplatesPage />} />
          <Route path="/jobs/:jobId" element={<h1>Job detail 91</h1>} />
        </Routes>
      </MemoryRouter>,
    );

    const row = (await screen.findByText('Deploy db')).closest('tr');
    expect(row).toBeTruthy();
    fireEvent.click(within(row!).getByRole('button', { name: 'Launch' }));
    expect(await screen.findByText(/uses its saved inventory/i)).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));
    expect(await screen.findByText('Confirm resolved launch')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Launch job' }));

    expect(await screen.findByRole('heading', { name: 'Job detail 91' })).toBeTruthy();
    expect(mocks.getLaunchConfiguration).toHaveBeenCalledWith(12, expect.any(AbortSignal));
    expect(mocks.previewLaunch).toHaveBeenCalledWith(12, {}, expect.any(AbortSignal));
    expect(mocks.launchJob).toHaveBeenCalledWith({
      unified_job_template_id: 102,
      name: 'Deploy db',
    }, expect.any(AbortSignal));
  });
});
