import React, { useState } from 'react';
import { merchantAPI } from '../services/api';
import { useNavigate } from 'react-router-dom';

const MerchantCreate = () => {
  const [merchantData, setMerchantData] = useState({ name: '', contact_info: '' });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const navigate = useNavigate();

  const handleChange = (e) => {
    const { name, value } = e.target;
    setMerchantData(prev => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      setLoading(true);
      await merchantAPI.createMerchant(merchantData);
      navigate('/merchants');
    } catch (err) {
      setError('创建商户失败');
      console.error('Error creating merchant:', err);
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
          <input
            type="text"
            id="name"
            name="name"
            value={merchantData.name}
            onChange={handleChange}
            required
          />
        </div>
        <div className="form-group">
          <label htmlFor="contact_info">联系信息</label>
          <input
            type="text"
            id="contact_info"
            name="contact_info"
            value={merchantData.contact_info}
            onChange={handleChange}
            required
          />
        </div>
        <div className="form-actions">
          <button type="submit" disabled={loading} className="btn btn-primary">
            {loading ? '创建中...' : '创建商户'}
          </button>
          <button type="button" onClick={() => navigate('/merchants')} className="btn btn-secondary">
            取消
          </button>
        </div>
      </form>
    </div>
  );
};

export default MerchantCreate;