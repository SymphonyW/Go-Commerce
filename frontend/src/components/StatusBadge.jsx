const StatusBadge = ({ status, label }) => (
  <span className={`status-badge status-${status || 'default'}`}>
    <span aria-hidden="true" />
    {label || status}
  </span>
);

export default StatusBadge;
