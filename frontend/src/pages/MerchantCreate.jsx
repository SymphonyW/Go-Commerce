import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { merchantAPI } from '../services/api';

const MerchantCreate = () => {
  const [merchantData, setMerchantData] = useState({ name: '', contact_info: '' });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const role = localStorage.getItem('role');
  const canManageMerchants = role === 'merchant' || role === 'admin';

  if (!canManageMerchants) {
    return <div className="error-message">当前账户没有创建商户的权限。</div>;
  }

  const handleChange = (event) => {
    const { name, value } = event.target;
    setMerchantData((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    if (!merchantData.name.trim() || !merchantData.contact_info.trim()) {
      setError('请完整填写店铺名称和联系方式');
      return;
    }

    try {
      setLoading(true);
      setError('');
      await merchantAPI.createMerchant({
        name: merchantData.name.trim(),
        contact_info: merchantData.contact_info.trim(),
      });
      navigate(role === 'merchant' ? '/merchant' : '/merchants');
    } catch (err) {
      setError(err.response?.data?.error || '创建商户失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="merchant-create-container">
      <h1>创建新商户</h1>
      {error && <div className="error-message">{error}</div>}
      <form onSubmit={handleSubmit} className="merchant-form">
        <div className="form-group">
          <label htmlFor="name">商户名称</label>
          <input id="name" name="name" value={merchantData.name} onChange={handleChange} required />
        </div>
        <div className="form-group">
          <label htmlFor="contact_info">联系方式</label>
          <input id="contact_info" name="contact_info" value={merchantData.contact_info} onChange={handleChange} required />
        </div>
        <div className="form-actions">
          <button type="submit" disabled={loading} className="btn btn-primary">
            {loading ? '创建中...' : '创建商户'}
          </button>
          <button type="button" onClick={() => navigate(role === 'merchant' ? '/merchant' : '/merchants')} className="btn btn-secondary">
            取消
          </button>
        </div>
      </form>
    </div>
  );
};

export default MerchantCreate;
