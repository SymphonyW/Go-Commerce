const SectionCard = ({ title, subtitle, action, className = '', children }) => (
  <section className={`section-card ${className}`.trim()}>
    {(title || subtitle || action) && (
      <div className="section-card-header">
        <div>
          {title && <h2>{title}</h2>}
          {subtitle && <p>{subtitle}</p>}
        </div>
        {action && <div className="section-card-action">{action}</div>}
      </div>
    )}
    {children}
  </section>
);

export default SectionCard;
