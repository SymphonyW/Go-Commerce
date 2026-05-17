const ErrorState = ({ title = '页面暂时不可用', description, action, compact = false }) => (
  <div className={`state-card error-state ${compact ? 'compact' : ''}`.trim()}>
    <div className="state-icon" aria-hidden="true">
      !
    </div>
    <div>
      <h3>{title}</h3>
      {description && <p>{description}</p>}
    </div>
    {action && <div className="state-action">{action}</div>}
  </div>
);

export default ErrorState;
