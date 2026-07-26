import React, { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, KeyRound, Loader2, Play, RefreshCw, Server, ShieldCheck } from 'lucide-react';
import {
  ApiError,
  api,
  type JobLaunchConfiguration,
  type JobLaunchPreview,
  type JobLaunchResult,
  type LaunchPromptInput,
} from '../services/api';
import Button from './ui/Button';
import { FormErrorSummary, FormSection } from './ui/Form';
import { Input, Select, Textarea } from './ui/Input';
import Modal from './ui/Modal';
import { ErrorState, LoadingState } from './ui/StatePanel';

type Props = {
  isOpen: boolean;
  templateId: number | null;
  onClose: () => void;
  onLaunched: (job: JobLaunchResult) => void;
};

type FieldErrors = Partial<Record<'inventory' | 'credential' | 'variables' | 'limit' | string, string>>;

const messageFrom = (error: unknown, fallback: string) =>
  error instanceof Error && error.message ? error.message : fallback;

const isAbort = (error: unknown) =>
  error instanceof Error && error.name === 'AbortError';

const hasValue = (value: unknown) => value !== undefined && value !== null && value !== '';

const GovernedJobLaunchModal: React.FC<Props> = ({ isOpen, templateId, onClose, onLaunched }) => {
  const [configuration, setConfiguration] = useState<JobLaunchConfiguration | null>(null);
  const [preview, setPreview] = useState<JobLaunchPreview | null>(null);
  const [phase, setPhase] = useState<'configure' | 'confirm'>('configure');
  const [inventoryId, setInventoryId] = useState('');
  const [credentialId, setCredentialId] = useState('');
  const [surveyAnswers, setSurveyAnswers] = useState<Record<string, string>>({});
  const [variablesText, setVariablesText] = useState('');
  const [limit, setLimit] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [errors, setErrors] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [launching, setLaunching] = useState(false);
  const requestRef = useRef<AbortController | null>(null);
  const confirmationHeadingRef = useRef<HTMLHeadingElement>(null);

  const questions = configuration?.survey_spec?.spec ?? [];
  const hasPrompts = configuration
    ? Object.values(configuration.prompts).some(Boolean)
    : false;

  const resetFromConfiguration = (next: JobLaunchConfiguration) => {
    setConfiguration(next);
    setInventoryId(
      next.defaults.inventory_id && next.inventories.some(item => item.id === next.defaults.inventory_id)
        ? String(next.defaults.inventory_id)
        : '',
    );
    setCredentialId(
      next.defaults.credential_id && next.credentials.some(item => item.id === next.defaults.credential_id)
        ? String(next.defaults.credential_id)
        : '',
    );
    const answers: Record<string, string> = {};
    for (const question of next.survey_spec?.spec ?? []) {
      answers[question.variable] = hasValue(question.default) ? String(question.default) : '';
    }
    setSurveyAnswers(answers);
    setVariablesText(
      Object.keys(next.defaults.extra_vars ?? {}).length
        ? JSON.stringify(next.defaults.extra_vars, null, 2)
        : '',
    );
    setLimit(next.defaults.limit ?? '');
  };

  useEffect(() => {
    if (!isOpen || !templateId) return;
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    setLoading(true);
    setConfiguration(null);
    setPreview(null);
    setPhase('configure');
    setErrors([]);
    setFieldErrors({});
    api.getJobLaunchConfiguration(templateId, controller.signal)
      .then(resetFromConfiguration)
      .catch(error => {
        if (!isAbort(error)) setErrors([messageFrom(error, 'Could not load launch configuration.')]);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [isOpen, templateId]);

  useEffect(() => {
    if (phase === 'confirm') confirmationHeadingRef.current?.focus();
  }, [phase, preview]);

  const close = () => {
    requestRef.current?.abort();
    onClose();
  };

  const updateAnswer = (variable: string, value: string) => {
    setSurveyAnswers(current => ({ ...current, [variable]: value }));
    setFieldErrors(current => ({ ...current, [variable]: undefined }));
    setErrors([]);
    setPreview(null);
  };

  const buildInput = (): LaunchPromptInput | null => {
    if (!configuration) return null;
    const nextFieldErrors: FieldErrors = {};
    const input: LaunchPromptInput = {};

    if (configuration.prompts.inventory && inventoryId) input.inventory_id = Number(inventoryId);
    if (configuration.prompts.credential && credentialId) input.credential_id = Number(credentialId);
    if (configuration.prompts.limit) {
      if (limit.length > 512 || /[\0\r\n]/.test(limit)) nextFieldErrors.limit = 'Use a single host pattern of 512 characters or fewer.';
      else input.limit = limit.trim();
    }

    if (configuration.prompts.survey) {
      const answers: Record<string, unknown> = {};
      for (const question of questions) {
        const raw = surveyAnswers[question.variable] ?? '';
        if (!raw && question.required && !hasValue(question.default)) {
          nextFieldErrors[question.variable] = `${question.question_name || question.variable} is required.`;
          continue;
        }
        if (!raw) continue;
        if (question.type === 'integer') {
          if (!/^-?\d+$/.test(raw.trim())) {
            nextFieldErrors[question.variable] = `${question.question_name || question.variable} must be a whole number.`;
            continue;
          }
          answers[question.variable] = Number(raw);
        } else {
          answers[question.variable] = raw;
        }
      }
      input.extra_vars = answers;
    } else if (configuration.prompts.variables) {
      if (variablesText.trim()) {
        try {
          const parsed = JSON.parse(variablesText);
          if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
            nextFieldErrors.variables = 'Variables must be a JSON object.';
          } else {
            input.extra_vars = parsed;
          }
        } catch {
          nextFieldErrors.variables = 'Variables must be valid JSON.';
        }
      } else {
        input.extra_vars = {};
      }
    }

    setFieldErrors(nextFieldErrors);
    const validationErrors = Object.values(nextFieldErrors).filter((value): value is string => Boolean(value));
    setErrors(validationErrors);
    return validationErrors.length ? null : input;
  };

  const refreshConfigurationAfterForbidden = async (signal: AbortSignal) => {
    if (!templateId) return;
    try {
      const fresh = await api.getJobLaunchConfiguration(templateId, signal);
      const nextErrors: FieldErrors = {};
      if (inventoryId && !fresh.inventories.some(item => String(item.id) === inventoryId)) {
        setInventoryId('');
        nextErrors.inventory = 'This inventory is no longer available to you. Choose another inventory.';
      }
      if (credentialId && !fresh.credentials.some(item => String(item.id) === credentialId)) {
        setCredentialId('');
        nextErrors.credential = 'This credential is no longer available to you. Choose another credential.';
      }
      setConfiguration(fresh);
      setFieldErrors(current => ({ ...current, ...nextErrors }));
    } catch (error) {
      if (!isAbort(error)) {
        setErrors(current => [...current, 'Available launch choices could not be refreshed.']);
      }
    }
  };

  const resolvePreview = async () => {
    if (!templateId) return;
    const input = buildInput();
    if (!input) return;
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    setPreviewing(true);
    setErrors([]);
    try {
      const resolved = await api.previewJobLaunch(templateId, input, controller.signal);
      setPreview(resolved);
      setPhase('confirm');
    } catch (error) {
      if (isAbort(error)) return;
      if ((error instanceof ApiError && error.status === 403) || (error as { status?: number })?.status === 403) {
        setErrors(['Your access to a selected inventory or credential changed. Review the available choices and try again.']);
        await refreshConfigurationAfterForbidden(controller.signal);
      } else {
        setErrors([messageFrom(error, 'The launch preview could not be resolved.')]);
      }
    } finally {
      if (!controller.signal.aborted) setPreviewing(false);
    }
  };

  const submitPreview = (event: FormEvent) => {
    event.preventDefault();
    void resolvePreview();
  };

  const launch = async () => {
    if (!configuration || !templateId) return;
    const input = buildInput();
    if (!input) {
      setPhase('configure');
      return;
    }
    const controller = new AbortController();
    requestRef.current?.abort();
    requestRef.current = controller;
    setLaunching(true);
    setErrors([]);
    try {
      const job = await api.launchJob({
        unified_job_template_id: configuration.template.unified_job_template_id,
        name: configuration.template.name,
        ...input,
      }, controller.signal);
      onLaunched(job);
    } catch (error) {
      if (!isAbort(error)) setErrors([messageFrom(error, 'Launch failed. Review the resolved inputs and try again.')]);
    } finally {
      if (!controller.signal.aborted) setLaunching(false);
    }
  };

  const maskedVariables = useMemo(() => {
    const values = { ...(preview?.extra_vars ?? {}) };
    for (const question of questions) {
      if (question.type === 'password' && Object.prototype.hasOwnProperty.call(values, question.variable)) {
        values[question.variable] = '••••••••';
      }
    }
    return values;
  }, [preview, questions]);

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      title={configuration ? `Launch ${configuration.template.name}` : 'Launch job template'}
      size="lg"
    >
      {loading && <LoadingState label="Loading authorized launch options" />}
      {!loading && !configuration && (
        <div className="space-y-4">
          <FormErrorSummary title="Launch configuration unavailable" errors={errors} />
          <ErrorState title="This template cannot be prepared for launch" description="Close the dialog and try again." />
        </div>
      )}
      {!loading && configuration && phase === 'configure' && (
        <form onSubmit={submitPreview} className="space-y-5" noValidate>
          <div className="flex items-center justify-between gap-4 border-b border-line pb-3">
            <div>
              <p className="text-sm font-medium text-ink">Configure launch</p>
              <p className="mt-0.5 text-xs text-mut">Only options allowed by this template and your current access are shown.</p>
            </div>
            <span className="font-mono text-[11px] text-dim">Review required</span>
          </div>

          <FormErrorSummary errors={errors} />

          {!hasPrompts && (
            <p className="rounded-lg bg-panel2 px-3 py-2.5 text-sm text-mut">
              This template uses its saved inventory, credential, variables, and host limit.
            </p>
          )}

          {(configuration.prompts.inventory || configuration.prompts.credential) && (
            <FormSection title="Execution resources" description="Choices are filtered by organization and current Use permission.">
              <div className="grid gap-4 sm:grid-cols-2">
                {configuration.prompts.inventory && (
                  <Select
                    label="Inventory"
                    value={inventoryId}
                    error={fieldErrors.inventory}
                    onChange={event => {
                      setInventoryId(event.target.value);
                      setFieldErrors(current => ({ ...current, inventory: undefined }));
                      setPreview(null);
                    }}
                  >
                    <option value="">Use template default</option>
                    {configuration.inventories.map(inventory => (
                      <option key={inventory.id} value={inventory.id}>{inventory.name}</option>
                    ))}
                  </Select>
                )}
                {configuration.prompts.credential && (
                  <Select
                    label="Machine credential"
                    value={credentialId}
                    error={fieldErrors.credential}
                    hint="Names identify sealed credential references. Secret values are never sent to this browser."
                    onChange={event => {
                      setCredentialId(event.target.value);
                      setFieldErrors(current => ({ ...current, credential: undefined }));
                      setPreview(null);
                    }}
                  >
                    <option value="">Use template default</option>
                    {configuration.credentials.map(credential => (
                      <option key={credential.id} value={credential.id}>{credential.name} · {credential.credential_type}</option>
                    ))}
                  </Select>
                )}
              </div>
            </FormSection>
          )}

          {configuration.prompts.survey && (
            <FormSection
              title={configuration.survey_spec?.name || 'Launch survey'}
              description={configuration.survey_spec?.description || 'Provide the structured values required by this template.'}
            >
              <div className="space-y-4">
                {questions.map(question => {
                  const label = question.question_name || question.variable;
                  const common = {
                    key: question.variable,
                    label,
                    required: question.required,
                    value: surveyAnswers[question.variable] ?? '',
                    error: fieldErrors[question.variable],
                    onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) =>
                      updateAnswer(question.variable, event.target.value),
                  };
                  if (question.type === 'textarea') return <Textarea {...common} rows={3} />;
                  if (question.type === 'multiplechoice') {
                    return (
                      <Select {...common}>
                        <option value="">Select an option</option>
                        {(question.choices || '').split('\n').map(choice => choice.trim()).filter(Boolean).map(choice => (
                          <option key={choice} value={choice}>{choice}</option>
                        ))}
                      </Select>
                    );
                  }
                  return (
                    <Input
                      {...common}
                      type={question.type === 'password' ? 'password' : 'text'}
                      inputMode={question.type === 'integer' ? 'numeric' : undefined}
                      autoComplete={question.type === 'password' ? 'new-password' : undefined}
                    />
                  );
                })}
              </div>
            </FormSection>
          )}

          {!configuration.prompts.survey && configuration.prompts.variables && (
            <FormSection title="Variables" description="Supply a JSON object. Template defaults remain in effect and these keys override them.">
              <Textarea
                label="Launch variables (JSON)"
                rows={6}
                className="font-mono text-xs"
                value={variablesText}
                error={fieldErrors.variables}
                onChange={event => {
                  setVariablesText(event.target.value);
                  setFieldErrors(current => ({ ...current, variables: undefined }));
                  setPreview(null);
                }}
                spellCheck={false}
              />
            </FormSection>
          )}

          {configuration.prompts.limit && (
            <FormSection title="Target">
              <Input
                label="Host limit"
                hint="An Ansible host pattern evaluated by the executor at run time."
                placeholder="web:&production"
                value={limit}
                error={fieldErrors.limit}
                onChange={event => {
                  setLimit(event.target.value);
                  setFieldErrors(current => ({ ...current, limit: undefined }));
                  setPreview(null);
                }}
              />
            </FormSection>
          )}

          <div className="flex flex-wrap justify-end gap-3 border-t border-line pt-4">
            <Button type="button" variant="secondary" onClick={close} disabled={previewing}>Cancel</Button>
            <Button type="submit" disabled={previewing} aria-busy={previewing} icon={previewing ? <Loader2 size={14} className="animate-spin" /> : <ShieldCheck size={14} />}>
              {previewing ? 'Resolving preview…' : 'Review launch'}
            </Button>
          </div>
        </form>
      )}

      {!loading && configuration && phase === 'confirm' && preview && (
        <div className="space-y-5">
          <div className="border-b border-line pb-3">
            <button type="button" onClick={() => { setPhase('configure'); setErrors([]); }} className="mb-2 inline-flex items-center gap-1.5 text-xs text-mut hover:text-ink">
              <ArrowLeft size={13} /> Edit launch options
            </button>
            <h3 ref={confirmationHeadingRef} tabIndex={-1} className="text-sm font-semibold text-ink focus:outline-none">Confirm resolved launch</h3>
            <p className="mt-1 text-xs text-mut">Praetor resolved these values now and will authorize them again when you launch.</p>
          </div>

          <FormErrorSummary title="Launch was not created" errors={errors} />

          <dl className="divide-y divide-line border-y border-line text-sm">
            <div className="grid gap-1 py-3 sm:grid-cols-[150px_1fr]">
              <dt className="text-mut">Template</dt>
              <dd className="font-medium text-ink">{preview.template.name}</dd>
            </div>
            <div className="grid gap-1 py-3 sm:grid-cols-[150px_1fr]">
              <dt className="flex items-center gap-1.5 text-mut"><Server size={13} /> Inventory</dt>
              <dd className="text-ink2">
                {preview.inventory ? (
                  <>
                    <span className="font-medium text-ink">{preview.inventory.name}</span>
                    <span className="ml-2 font-mono text-xs text-mut">{preview.inventory_host_count} enabled host{preview.inventory_host_count === 1 ? '' : 's'}</span>
                    {preview.inventory_host_sample.length > 0 && (
                      <p className="mt-1 font-mono text-xs text-mut">
                        Sample: {preview.inventory_host_sample.join(', ')}
                        {preview.inventory_host_count > preview.inventory_host_sample.length ? ', …' : ''}
                      </p>
                    )}
                  </>
                ) : 'No inventory selected'}
              </dd>
            </div>
            <div className="grid gap-1 py-3 sm:grid-cols-[150px_1fr]">
              <dt className="flex items-center gap-1.5 text-mut"><KeyRound size={13} /> Credential</dt>
              <dd className="text-ink2">
                {preview.credential ? (
                  <>
                    <span className="font-medium text-ink">{preview.credential.name}</span>
                    <span className="ml-2 text-xs text-mut">{preview.credential.credential_type}</span>
                    <p className="mt-1 text-xs text-mut">The secret remains sealed and is never displayed.</p>
                  </>
                ) : 'No credential selected'}
              </dd>
            </div>
            <div className="grid gap-1 py-3 sm:grid-cols-[150px_1fr]">
              <dt className="text-mut">Target limit</dt>
              <dd className="font-mono text-xs text-ink2">{preview.limit || 'No additional limit'}</dd>
            </div>
            {preview.approval_team && (
              <div className="grid gap-1 py-3 sm:grid-cols-[150px_1fr]">
                <dt className="text-mut">Approval team</dt>
                <dd className="font-medium text-ink">{preview.approval_team.name}</dd>
              </div>
            )}
          </dl>

          <FormSection title="Effective variables" description="Password survey answers are masked in this confirmation.">
            <pre className="max-h-44 overflow-auto rounded-lg bg-panel2 p-3 font-mono text-xs leading-relaxed text-ink2">
              {Object.keys(maskedVariables).length ? JSON.stringify(maskedVariables, null, 2) : 'No launch variables'}
            </pre>
          </FormSection>

          <div className="flex flex-wrap justify-end gap-3 border-t border-line pt-4">
            <Button type="button" variant="secondary" onClick={() => { setPhase('configure'); setErrors([]); }} disabled={launching || previewing}>Back</Button>
            <Button type="button" variant="secondary" onClick={() => void resolvePreview()} disabled={launching || previewing} icon={previewing ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}>
              {previewing ? 'Refreshing…' : 'Refresh preview'}
            </Button>
            <Button type="button" onClick={() => void launch()} disabled={launching || previewing} aria-busy={launching} icon={launching ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}>
              {launching ? 'Launching…' : 'Launch job'}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
};

export default GovernedJobLaunchModal;
