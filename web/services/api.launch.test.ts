import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api } from './api';

const jsonResponse = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
  status,
  headers: { 'Content-Type': 'application/json' },
});

afterEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe('governed launch API contracts', () => {
  it('keeps configuration, preview, and launch as distinct abortable requests', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(jsonResponse({
        template: { id: 7, unified_job_template_id: 70, name: 'Deploy edge', organization_id: 5 },
        prompts: { inventory: true, credential: false, variables: false, limit: true, survey: false },
        defaults: { inventory_id: 21, extra_vars: {}, limit: '' },
        inventories: [{ id: 21, name: 'Edge fleet', kind: 'static' }],
        credentials: [],
      }))
      .mockResolvedValueOnce(jsonResponse({
        template: { id: 7, unified_job_template_id: 70, name: 'Deploy edge' },
        inventory: { id: 21, name: 'Edge fleet', kind: 'static' },
        extra_vars: {},
        limit: 'edge-01',
        inventory_host_count: 1,
        inventory_host_sample: ['edge-01'],
        limit_applied_at_execution: true,
      }))
      .mockResolvedValueOnce(jsonResponse({ id: 91, status: 'pending' }, 201));
    const signal = new AbortController().signal;

    await api.getJobLaunchConfiguration(7, signal);
    await api.previewJobLaunch(7, { inventory_id: 21, limit: 'edge-01' }, signal);
    const job = await api.launchJob({
      unified_job_template_id: 70,
      name: 'Deploy edge',
      inventory_id: 21,
      limit: 'edge-01',
    }, signal);

    expect(job).toEqual({ id: 91, status: 'pending' });
    expect(fetchMock).toHaveBeenNthCalledWith(1,
      '/api/v1/job-templates/7/launch-configuration',
      expect.objectContaining({ signal }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(2,
      '/api/v1/job-templates/7/launch-preview',
      expect.objectContaining({ method: 'POST', signal }),
    );
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({
      inventory_id: 21,
      limit: 'edge-01',
    });
    expect(fetchMock).toHaveBeenNthCalledWith(3,
      '/api/v1/jobs',
      expect.objectContaining({ method: 'POST', signal }),
    );
  });

  it('preserves the response status for authorization-aware recovery', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(jsonResponse({
      error: 'selected launch resource is unavailable',
    }, 403));

    await expect(api.previewJobLaunch(7, { inventory_id: 999 }))
      .rejects.toMatchObject({
        name: 'ApiError',
        message: 'selected launch resource is unavailable',
        status: 403,
      } satisfies Partial<ApiError>);
  });
});
