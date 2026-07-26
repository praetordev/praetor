import React from 'react';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import GovernedJobLaunchModal from './GovernedJobLaunchModal';
import { api, type JobLaunchConfiguration, type JobLaunchPreview } from '../services/api';

const mocks = vi.hoisted(() => ({
  getConfiguration: vi.fn(),
  preview: vi.fn(),
  launch: vi.fn(),
}));

vi.mock('../services/api', () => {
  class ApiError extends Error {
    status: number;
    constructor(message: string, status: number) {
      super(message);
      this.status = status;
    }
  }
  return {
    ApiError,
    api: {
      getJobLaunchConfiguration: mocks.getConfiguration,
      previewJobLaunch: mocks.preview,
      launchJob: mocks.launch,
    },
  };
});

const configuration = (patch: Partial<JobLaunchConfiguration> = {}): JobLaunchConfiguration => ({
  template: { id: 7, unified_job_template_id: 70, name: 'Deploy edge', organization_id: 5 },
  prompts: { inventory: true, credential: true, variables: true, limit: true, survey: false },
  defaults: { inventory_id: 21, credential_id: 31, extra_vars: { region: 'eu-west' }, limit: 'edge:&enabled' },
  inventories: [
    { id: 21, name: 'Edge fleet', kind: 'static' },
    { id: 22, name: 'Canary fleet', kind: 'static' },
  ],
  credentials: [
    { id: 31, name: 'Edge SSH', credential_type_id: 4, credential_type: 'Machine' },
  ],
  ...patch,
});

const preview = (patch: Partial<JobLaunchPreview> = {}): JobLaunchPreview => ({
  template: { id: 7, unified_job_template_id: 70, name: 'Deploy edge' },
  inventory: { id: 21, name: 'Edge fleet', kind: 'static' },
  credential: { id: 31, name: 'Edge SSH', credential_type_id: 4, credential_type: 'Machine' },
  extra_vars: { region: 'eu-west' },
  limit: 'edge:&enabled',
  inventory_host_count: 12,
  inventory_host_sample: ['edge-01', 'edge-02'],
  limit_applied_at_execution: true,
  ...patch,
});

const renderModal = (props: Partial<React.ComponentProps<typeof GovernedJobLaunchModal>> = {}) => {
  const onClose = vi.fn();
  const onLaunched = vi.fn();
  render(
    <GovernedJobLaunchModal
      isOpen
      templateId={7}
      onClose={onClose}
      onLaunched={onLaunched}
      {...props}
    />,
  );
  return { onClose, onLaunched };
};

beforeEach(() => {
  mocks.getConfiguration.mockReset();
  mocks.preview.mockReset();
  mocks.launch.mockReset();
  mocks.getConfiguration.mockResolvedValue(configuration());
  mocks.preview.mockResolvedValue(preview());
  mocks.launch.mockResolvedValue({ id: 91, status: 'pending' });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('governed job launch form', () => {
  it('renders only server-enabled authorized prompts and never exposes credential secrets', async () => {
    renderModal();

    expect(await screen.findByLabelText('Inventory')).toBeTruthy();
    expect(screen.getByLabelText('Machine credential')).toBeTruthy();
    expect(screen.getByLabelText('Launch variables (JSON)')).toBeTruthy();
    expect(screen.getByLabelText('Host limit')).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Canary fleet' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Edge SSH · Machine' })).toBeTruthy();
    expect(screen.queryByText(/private key|password value/i)).toBeNull();

    cleanup();
    mocks.getConfiguration.mockResolvedValue(configuration({
      prompts: { inventory: false, credential: false, variables: false, limit: false, survey: false },
      inventories: [],
      credentials: [],
    }));
    renderModal();

    expect(await screen.findByText(/uses its saved inventory/i)).toBeTruthy();
    expect(screen.queryByLabelText('Inventory')).toBeNull();
    expect(screen.queryByLabelText('Machine credential')).toBeNull();
    expect(screen.queryByLabelText('Launch variables (JSON)')).toBeNull();
    expect(screen.queryByLabelText('Host limit')).toBeNull();
  });

  it('uses structured surveys instead of raw JSON and validates answers before preview', async () => {
    mocks.getConfiguration.mockResolvedValue(configuration({
      prompts: { inventory: false, credential: false, variables: true, limit: false, survey: true },
      survey_spec: {
        name: 'Release inputs',
        spec: [
          { variable: 'release', question_name: 'Release version', type: 'text', required: true },
          { variable: 'replicas', question_name: 'Replica count', type: 'integer', required: true },
          { variable: 'token', question_name: 'Deployment token', type: 'password', required: false },
          { variable: 'channel', question_name: 'Channel', type: 'multiplechoice', required: false, choices: 'stable\ncanary' },
        ],
      },
    }));
    renderModal();

    await screen.findByText('Release inputs');
    expect(screen.queryByLabelText('Launch variables (JSON)')).toBeNull();
    fireEvent.change(screen.getByLabelText(/Replica count/), { target: { value: 'not-a-number' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Release version is required');
    expect(alert.textContent).toContain('Replica count must be a whole number');
    expect(document.activeElement).toBe(alert);
    expect(mocks.preview).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/Release version/), { target: { value: '2.4.0' } });
    fireEvent.change(screen.getByLabelText(/Replica count/), { target: { value: '4' } });
    fireEvent.change(screen.getByLabelText('Deployment token'), { target: { value: 'sealed-value' } });
    fireEvent.change(screen.getByLabelText('Channel'), { target: { value: 'canary' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));

    await screen.findByText('Confirm resolved launch');
    expect(mocks.preview).toHaveBeenCalledWith(7, {
      extra_vars: { release: '2.4.0', replicas: 4, token: 'sealed-value', channel: 'canary' },
    }, expect.any(AbortSignal));
  });

  it('shows the resolved preview, masks password answers, and refreshes stale host facts', async () => {
    mocks.getConfiguration.mockResolvedValue(configuration({
      prompts: { inventory: true, credential: true, variables: true, limit: true, survey: true },
      survey_spec: {
        spec: [{ variable: 'token', question_name: 'Deployment token', type: 'password', required: true }],
      },
    }));
    mocks.preview
      .mockResolvedValueOnce(preview({ extra_vars: { token: 'sealed-value', region: 'eu-west' } }))
      .mockResolvedValueOnce(preview({
        extra_vars: { token: 'sealed-value', region: 'eu-west' },
        inventory_host_count: 13,
        inventory_host_sample: ['edge-01', 'edge-02', 'edge-03'],
      }));
    renderModal();

    fireEvent.change(await screen.findByLabelText(/Deployment token/), { target: { value: 'sealed-value' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));

    const heading = await screen.findByText('Confirm resolved launch');
    expect(document.activeElement).toBe(heading);
    expect(screen.getByText('12 enabled hosts')).toBeTruthy();
    expect(screen.getByText('Edge SSH')).toBeTruthy();
    expect(screen.getByText(/secret remains sealed/i)).toBeTruthy();
    expect(screen.getByText(/••••••••/)).toBeTruthy();
    expect(screen.queryByText('sealed-value')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Refresh preview' }));
    expect(await screen.findByText('13 enabled hosts')).toBeTruthy();
    expect(mocks.preview).toHaveBeenCalledTimes(2);
  });

  it('refreshes choices after an authorization change while retaining valid answers', async () => {
    mocks.preview.mockRejectedValue(Object.assign(new Error('forbidden'), { status: 403 }));
    mocks.getConfiguration
      .mockResolvedValueOnce(configuration())
      .mockResolvedValueOnce(configuration({
        defaults: { credential_id: 31, extra_vars: { region: 'eu-west' }, limit: 'edge:&enabled' },
        inventories: [{ id: 22, name: 'Canary fleet', kind: 'static' }],
      }));
    renderModal();

    await screen.findByLabelText('Inventory');
    fireEvent.change(screen.getByLabelText('Host limit'), { target: { value: 'edge-02' } });
    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));

    expect(await screen.findByText(/access to a selected inventory or credential changed/i)).toBeTruthy();
    await waitFor(() => expect((screen.getByLabelText('Inventory') as HTMLSelectElement).value).toBe(''));
    expect(screen.getByText(/inventory is no longer available/i)).toBeTruthy();
    expect((screen.getByLabelText('Host limit') as HTMLInputElement).value).toBe('edge-02');
    expect((screen.getByLabelText('Machine credential') as HTMLSelectElement).value).toBe('31');
  });

  it('retains the resolved launch after a failed submission and succeeds on retry', async () => {
    mocks.launch
      .mockRejectedValueOnce(new Error('executor unavailable'))
      .mockResolvedValueOnce({ id: 91, status: 'pending' });
    const { onLaunched } = renderModal();

    await screen.findByLabelText('Inventory');
    fireEvent.click(screen.getByRole('button', { name: 'Review launch' }));
    await screen.findByText('Confirm resolved launch');
    fireEvent.click(screen.getByRole('button', { name: 'Launch job' }));

    expect(await screen.findByText('executor unavailable')).toBeTruthy();
    expect(screen.getByText('Edge fleet')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Launch job' }));
    await waitFor(() => expect(onLaunched).toHaveBeenCalledWith({ id: 91, status: 'pending' }));
    expect(mocks.launch).toHaveBeenCalledTimes(2);
  });

  it('aborts active requests when cancelled', async () => {
    let observedSignal: AbortSignal | undefined;
    mocks.getConfiguration.mockImplementation((_id: number, signal: AbortSignal) => {
      observedSignal = signal;
      return new Promise(() => undefined);
    });
    const { onClose } = renderModal();

    await screen.findByText(/Loading authorized launch options/);
    fireEvent.click(screen.getByRole('button', { name: 'Close dialog' }));

    expect(observedSignal?.aborted).toBe(true);
    expect(onClose).toHaveBeenCalledOnce();
  });
});
