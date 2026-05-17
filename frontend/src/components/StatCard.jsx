const StatCard = ({ label, value, helper, tone = 'primary', icon }) => (
  <article className={`stat-card tone-${tone}`}>
    <div className="stat-card-icon" aria-hidden="true">
      {icon}
    </div>
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
      {helper && <small>{helper}</small>}
    </div>
  </article>
);

export default StatCard;
