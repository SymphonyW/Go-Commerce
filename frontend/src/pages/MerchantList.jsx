import React, { useState, useEffect } from 'react';
import { merchantAPI } from '../services/api';
import { Link } from 'react-router-dom';

const MerchantList = () => {
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchMerchants();
  }, []);

  const fetchMerchants = async () => {
    try {
      setLoading(true);
      const data = await merchantAPI.listMerchants();
      setMerchants(data.merchants || []);
    } catch (err) {
      setError('获取商户列表失败');
      console.error('Error fetching merchants:', err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="merchant-list-container">
      <h1>商户管理</h1>
      
      {error && <div className="error-message">{error}</div>}
      
      <div className="page-actions">
        <Link to="/merchants/create" className="btn btn-primary">
          创建新商户
        </Link>
      </div>
      
      <div className="merchants-list">
        <h2>商户列表</h2>
        {loading ? (
          <p>加载中...</p>
        ) : (
          <div className="merchants-grid">
            {merchants.map((merchant) => (
              <div key={merchant.id} className="merchant-card">
                <h3>{merchant.name}</h3>
                <p>商户ID: {merchant.id}</p>
                <p>联系信息: {merchant.contact_info}</p>
                <p>创建时间: {new Date(merchant.created_at).toLocaleString()}</p>
                <div className="card-actions">
                  <Link to={`/merchants/${merchant.id}/products`} className="btn btn-sm">
                    管理产品
                  </Link>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default MerchantList;