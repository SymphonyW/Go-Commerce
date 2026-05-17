const EmptyState = ({ title, description, action, compact = false, icon = '◇' }) => (
  <div className={`state-card empty-state ${compact ? 'compact' : ''}`.trim()}>
    <div className="state-icon" aria-hidden="true">
      {icon}
    </div>
    <div>
      <h3>{title}</h3>
      {description && <p>{description}</p>}
    </div>
    {action && <div className="state-action">{action}</div>}
  </div>
);

export default EmptyState;
